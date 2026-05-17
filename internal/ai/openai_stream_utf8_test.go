package ai

import "testing"

// Background: 一个 3 字节中文字符（如"未"= 0xE6 0x9C 0xAA）在 LLM stream
// 里被任意切分时，原始实现会让 Go json.Marshal 把末尾不完整字节替换成
// U+FFFD（��）发到前端。utf8DeltaBuffer 必须把切碎的字符还原为完整。
//
// 注：测试不依赖网络，直接验证 buffer 语义。
func TestUtf8DeltaBuffer_NoSplit(t *testing.T) {
	var b utf8DeltaBuffer
	out := b.Push("帮我做志愿")
	if out != "帮我做志愿" {
		t.Fatalf("expected full passthrough, got %q", out)
	}
	if tail := b.Flush(); tail != "" {
		t.Fatalf("expected empty flush, got %q", tail)
	}
}

func TestUtf8DeltaBuffer_SplitInsideRune(t *testing.T) {
	// "未" = 0xE6 0x9C 0xAA (3 bytes)
	// chunk1: "我说" + 0xE6 (lead byte of 未)
	// chunk2: 0x9C 0xAA (剩余 2 bytes) + "来"
	chunk1 := "我说" + string([]byte{0xE6})
	chunk2 := string([]byte{0x9C, 0xAA}) + "来"

	var b utf8DeltaBuffer
	out1 := b.Push(chunk1)
	if out1 != "我说" {
		t.Fatalf("chunk1: expected only complete prefix %q, got %q", "我说", out1)
	}

	out2 := b.Push(chunk2)
	if out2 != "未来" {
		t.Fatalf("chunk2: expected merged %q, got %q", "未来", out2)
	}
}

func TestUtf8DeltaBuffer_SplitTwoConsecutive(t *testing.T) {
	// 两个连续 chunk 都在 rune 中间切：chunk2 仍不完整，chunk3 才补全。
	// "好" = 0xE5 0xA5 0xBD
	chunk1 := "ok " + string([]byte{0xE5})           // 起始字节
	chunk2 := string([]byte{0xA5})                   // 第二字节
	chunk3 := string([]byte{0xBD}) + " done"         // 第三字节 + 后续

	var b utf8DeltaBuffer
	if out := b.Push(chunk1); out != "ok " {
		t.Fatalf("chunk1 = %q, want %q", out, "ok ")
	}
	if out := b.Push(chunk2); out != "" {
		t.Fatalf("chunk2 expected empty (still incomplete), got %q", out)
	}
	if out := b.Push(chunk3); out != "好 done" {
		t.Fatalf("chunk3 = %q, want %q", out, "好 done")
	}
}

func TestUtf8DeltaBuffer_FlushIncompleteAtEnd(t *testing.T) {
	// stream 结束时如果 pending 中还有不完整字节，必须 emit 出去
	// （会变成 U+FFFD，但不能默默吞掉——否则用户看不到任何尾部内容）。
	var b utf8DeltaBuffer
	out := b.Push("hello " + string([]byte{0xE6, 0x9C})) // 不完整 3 字节字符
	if out != "hello " {
		t.Fatalf("push returned %q, want %q", out, "hello ")
	}
	tail := b.Flush()
	if tail == "" {
		t.Fatalf("flush must emit pending bytes, got empty")
	}
}

func TestUtf8DeltaBuffer_PureAscii(t *testing.T) {
	// 纯 ASCII 不该被缓冲——每个 ASCII 字符自身就是完整 rune。
	var b utf8DeltaBuffer
	out := b.Push("hello world")
	if out != "hello world" {
		t.Fatalf("ascii passthrough = %q", out)
	}
	if len(b.pending) != 0 {
		t.Fatalf("ascii should not leave pending bytes, got %d", len(b.pending))
	}
}
