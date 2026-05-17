package ai

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

// ErrTurnAlreadyRunning is returned when a caller tries to start a turn
// for a conversation that already has an in-flight one. Frontend should
// instead subscribe to the existing turn via the stream endpoint.
var ErrTurnAlreadyRunning = errors.New("a turn for this conversation is already running")

// turnRetention 控制 turn 结束后在 manager 中存活多久——给那些"完成
// 之后才切回来"的客户端一个补窗，能从 backlog 重放完整事件流。
const turnRetention = 3 * time.Minute

// turnRunDeadline 是 detached background 跑 agent 的硬上限。超过这个
// 时间无论 agent 是否完成都会被取消——避免泄漏 goroutine。
const turnRunDeadline = 10 * time.Minute

// Turn 是"一个 conversation 的当前在跑或刚跑完的 agent 回合"。
//
// 它把 SSE 事件流从 HTTP 连接里剥离出来：agent 真正跑在 goroutine
// 里，事件先写入 in-memory backlog 并同时广播给当前活跃的订阅者。
// 客户端断开（切对话 / 网络抖动）只是 unsubscribe，agent 继续跑完。
// 客户端重连时调 Subscribe()：先收到完整 backlog，再追平实时增量。
type Turn struct {
	convID int64
	ctx    context.Context
	cancel context.CancelFunc

	mu       sync.Mutex
	events   []SSEEvent
	subs     map[int]chan SSEEvent
	nextSub  int
	finished bool
	doneCh   chan struct{}
}

// Append 写入一个事件：先入 backlog（持久重放用），再 fan-out 给所有
// 订阅者。订阅 channel 满了就丢这条增量——backlog 里仍然有，订阅者
// 下次重连还能补到，避免一个慢消费者拖垮 agent 写入速度。
func (t *Turn) Append(ev SSEEvent) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.finished {
		return
	}
	t.events = append(t.events, ev)
	for _, ch := range t.subs {
		select {
		case ch <- ev:
		default:
			slog.Warn("turn subscriber buffer full; event dropped from realtime stream (still in backlog)",
				"conversationID", t.convID,
				"eventType", ev.Type,
			)
		}
	}
}

// markFinished 关闭所有订阅 channel，标记终态。订阅者收到 closed
// channel 后停止 select 循环；后续新订阅者只拿 backlog，不再分配 ch。
func (t *Turn) markFinished() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.finished {
		return
	}
	t.finished = true
	for _, ch := range t.subs {
		close(ch)
	}
	t.subs = nil
	close(t.doneCh)
}

// Subscribe 返回 (backlog, ch, unsubscribe)：
//
//   - backlog：调用时刻之前的全部事件，订阅方先按序回放给客户端
//   - ch：增量 channel；turn 已结束时返回 nil（订阅方不需要 select）
//   - unsubscribe：客户端断开时调用，从订阅列表里摘掉自己
//
// 这种"backlog + tail"模式让晚到的订阅者也能看到完整流，又能与正在
// 跑的 agent 保持低延迟同步——核心目的是支持"切走再切回"无缝续看。
func (t *Turn) Subscribe() ([]SSEEvent, <-chan SSEEvent, func()) {
	t.mu.Lock()
	defer t.mu.Unlock()
	backlog := append([]SSEEvent(nil), t.events...)
	if t.finished {
		return backlog, nil, func() {}
	}
	id := t.nextSub
	t.nextSub++
	// buffer=64 足以缓冲 LLM 文本流的爆发（一次 chunk 通常 5-20 字节，
	// 慢消费者也很难撑爆）；满了就走 Append 里的 drop fallback。
	ch := make(chan SSEEvent, 64)
	t.subs[id] = ch
	unsub := func() {
		t.mu.Lock()
		defer t.mu.Unlock()
		if existing, ok := t.subs[id]; ok {
			delete(t.subs, id)
			// 关掉自己的 channel，避免 Append 后续写入到悬空 sub。
			close(existing)
		}
	}
	return backlog, ch, unsub
}

// Done 返回一个会在 turn 终止时 close 的 channel——后台 goroutine
// 等待它来触发延迟清理。
func (t *Turn) Done() <-chan struct{} { return t.doneCh }

// Context 暴露 detached background ctx 给 agent 使用。它不绑定任何
// HTTP 请求，所以客户端断开不会让 agent 停。
func (t *Turn) Context() context.Context { return t.ctx }

// TurnManager 是 conversation→Turn 的进程级 in-memory 索引。同一时刻
// 一个 conversation 只允许有一个 active turn——多用户编辑的场景由调用
// 方上层处理，这里只做技术层互斥。
//
// 不持久化：进程重启 turn 全丢。可接受，因为 agent run 本来就是
// 短任务（≤ 10 分钟），重启时正好把跑了一半的 turn 当作 client cancel
// 处理——用户切回去能看到自己问过的话，点"继续生成"重试。
type TurnManager struct {
	mu    sync.Mutex
	turns map[int64]*Turn
}

func NewTurnManager() *TurnManager {
	return &TurnManager{turns: map[int64]*Turn{}}
}

// Start 创建并启动一个新 turn。如果该 conversation 已有 active turn
// 返回 ErrTurnAlreadyRunning——调用方应该让客户端去订阅现有的，而不
// 是排队启动新的（排队语义模糊：用户可能已经放弃旧问题）。
//
// run 是 agent 主体逻辑，会在新 goroutine 里被调用，传入的 Turn 对象
// 已经持有 detached ctx 和 backlog buffer——run 内部只需把事件丢进
// turn.Append、完成时 return。Return 后 manager 自动 markFinished
// 并安排 turnRetention 后清理。
func (m *TurnManager) Start(convID int64, run func(*Turn)) (*Turn, error) {
	m.mu.Lock()
	if existing, ok := m.turns[convID]; ok && !existing.isFinished() {
		m.mu.Unlock()
		return nil, ErrTurnAlreadyRunning
	}
	ctx, cancel := context.WithTimeout(context.Background(), turnRunDeadline)
	t := &Turn{
		convID: convID,
		ctx:    ctx,
		cancel: cancel,
		subs:   map[int]chan SSEEvent{},
		doneCh: make(chan struct{}),
	}
	m.turns[convID] = t
	m.mu.Unlock()

	go func() {
		defer func() {
			t.markFinished()
			t.cancel()
			// 延迟回收，让晚到的订阅者还能拿到 backlog 重放。
			time.AfterFunc(turnRetention, func() {
				m.mu.Lock()
				defer m.mu.Unlock()
				if cur, ok := m.turns[convID]; ok && cur == t {
					delete(m.turns, convID)
				}
			})
		}()
		// 防御性 panic 捕获：agent 跑挂了不能拖死整个进程；记录后正常
		// markFinished，订阅者会收到 close channel 自然退出。
		defer func() {
			if r := recover(); r != nil {
				slog.Error("turn run panicked",
					"conversationID", convID,
					"panic", r,
				)
				t.Append(SSEEvent{Type: "error", Content: "internal error during agent run"})
			}
		}()
		run(t)
	}()

	return t, nil
}

// Get 返回 convID 对应的 turn（可能为 nil）。已 finished 但未过保留期
// 的 turn 也会返回——订阅方拿到 backlog 重放后立即收到 closed ch。
func (m *TurnManager) Get(convID int64) *Turn {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.turns[convID]
}

func (t *Turn) isFinished() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.finished
}
