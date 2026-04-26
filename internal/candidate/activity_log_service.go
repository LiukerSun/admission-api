package candidate

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"admission-api/internal/platform/web"

	"github.com/redis/go-redis/v9"
)

// ActivityLogService defines activity log business operations.
type ActivityLogService interface {
	LogActivity(ctx context.Context, input CreateActivityInput) error
	ListActivities(ctx context.Context, filter ActivityFilter, page, pageSize int) (*ActivityLogListResponse, error)
	GetMyActivities(ctx context.Context, userID int64, page, pageSize int) (*ActivityLogListResponse, error)
	GetStats(ctx context.Context, targetType string, targetID int64) (*ActivityStatsResponse, error)
	DeleteByIDs(ctx context.Context, ids []int64) (int64, error)
	DeleteBefore(ctx context.Context, before time.Time) (int64, error)
}

type activityLogService struct {
	store ActivityLogStore
	rdb   *redis.Client
}

// NewActivityLogService creates a new activity log service.
func NewActivityLogService(store ActivityLogStore, rdb *redis.Client) ActivityLogService {
	return &activityLogService{store: store, rdb: rdb}
}

const activityLogQueueKey = "activity_log:queue"

func (s *activityLogService) LogActivity(ctx context.Context, input CreateActivityInput) error {
	data, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("marshal activity log: %w", err)
	}
	if err := s.rdb.LPush(ctx, activityLogQueueKey, data).Err(); err != nil {
		return fmt.Errorf("enqueue activity log: %w", err)
	}
	return nil
}

func (s *activityLogService) ListActivities(ctx context.Context, filter ActivityFilter, page, pageSize int) (*ActivityLogListResponse, error) {
	logs, total, err := s.store.List(ctx, filter, page, pageSize)
	if err != nil {
		return nil, err
	}
	return &ActivityLogListResponse{Logs: logs, Total: total}, nil
}

func (s *activityLogService) GetMyActivities(ctx context.Context, userID int64, page, pageSize int) (*ActivityLogListResponse, error) {
	logs, total, err := s.store.List(ctx, ActivityFilter{UserID: userID}, page, pageSize)
	if err != nil {
		return nil, err
	}
	return &ActivityLogListResponse{Logs: logs, Total: total}, nil
}

func (s *activityLogService) GetStats(ctx context.Context, targetType string, targetID int64) (*ActivityStatsResponse, error) {
	if targetType == "" || targetID <= 0 {
		return nil, web.NewError(web.ErrCodeBadRequest, "目标类型和目标ID不能为空")
	}
	count, err := s.store.GetStats(ctx, targetType, targetID)
	if err != nil {
		return nil, err
	}
	return &ActivityStatsResponse{TargetType: targetType, TargetID: targetID, Count: count}, nil
}

func (s *activityLogService) DeleteByIDs(ctx context.Context, ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, web.NewError(web.ErrCodeBadRequest, "ID列表不能为空")
	}
	return s.store.DeleteByIDs(ctx, ids)
}

func (s *activityLogService) DeleteBefore(ctx context.Context, before time.Time) (int64, error) {
	if before.IsZero() {
		return 0, web.NewError(web.ErrCodeBadRequest, "时间参数不能为空")
	}
	return s.store.DeleteBefore(ctx, before)
}

// ActivityLogConsumer consumes activity logs from Redis queue and flushes to DB.
type ActivityLogConsumer struct {
	store         ActivityLogStore
	rdb           *redis.Client
	batchSize     int
	flushInterval time.Duration
}

// NewActivityLogConsumer creates a new consumer.
func NewActivityLogConsumer(store ActivityLogStore, rdb *redis.Client) *ActivityLogConsumer {
	return &ActivityLogConsumer{
		store:         store,
		rdb:           rdb,
		batchSize:     100,
		flushInterval: 2 * time.Second,
	}
}

// Start begins consuming logs from Redis and writing to DB.
// Callers should wait on the returned channel for shutdown completion.
func (c *ActivityLogConsumer) Start(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	go c.run(ctx, done)
	return done
}

func (c *ActivityLogConsumer) run(ctx context.Context, done chan<- struct{}) {
	defer close(done)

	logCh := make(chan *CreateActivityInput, c.batchSize*2)

	// Reader goroutine: BRPOP from Redis and send to channel.
	go func() {
		for {
			select {
			case <-ctx.Done():
				close(logCh)
				return
			default:
			}

			result, err := c.rdb.BRPop(ctx, 2*time.Second, activityLogQueueKey).Result()
			if err != nil {
				if err != redis.Nil {
					// Context cancelled or other error.
					select {
					case <-ctx.Done():
						close(logCh)
						return
					case <-time.After(500 * time.Millisecond):
					}
				}
				continue
			}
			if len(result) < 2 {
				continue
			}

			var input CreateActivityInput
			if err := json.Unmarshal([]byte(result[1]), &input); err != nil {
				continue
			}

			select {
			case logCh <- &input:
			case <-ctx.Done():
				close(logCh)
				return
			}
		}
	}()

	// Flusher goroutine: batch accumulate and flush to DB.
	ticker := time.NewTicker(c.flushInterval)
	defer ticker.Stop()

	var buffer []*CreateActivityInput
	for {
		select {
		case <-ctx.Done():
			c.flush(buffer)
			return
		case log, ok := <-logCh:
			if !ok {
				c.flush(buffer)
				return
			}
			buffer = append(buffer, log)
			if len(buffer) >= c.batchSize {
				c.flush(buffer)
				buffer = buffer[:0]
			}
		case <-ticker.C:
			if len(buffer) > 0 {
				c.flush(buffer)
				buffer = buffer[:0]
			}
		}
	}
}

func (c *ActivityLogConsumer) flush(buffer []*CreateActivityInput) {
	if len(buffer) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := c.store.BatchCreate(ctx, buffer); err != nil {
		// In production, consider pushing failed items to a dead-letter queue.
		fmt.Printf("failed to flush activity logs: %v\n", err)
	}
}
