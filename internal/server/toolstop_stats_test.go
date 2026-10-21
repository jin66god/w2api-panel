package server

import "testing"

// issue #44：上游静默停摆的每一帧都带 "tool_calls":[]（空数组，不是字段缺失）。
// 判据必须按数组长度，否则会把这种回合判成「有工具调用」、WARN 永不触发。
func TestChatStatsEmptyToolCallsNotCounted(t *testing.T) {
	var s chatStatsReader
	s.parseSSELine(`data: {"choices":[{"delta":{"content":"。"},"finish_reason":null}]}`)
	s.parseSSELine(`data: {"choices":[{"delta":{"tool_calls":[]},"finish_reason":null}]}`)
	s.parseSSELine(`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`)
	s.parseSSELine(`data: [DONE]`)
	if n := s.ToolCalls(); n != 0 {
		t.Fatalf("空 tool_calls 数组不应计数，got %d", n)
	}
	if got := s.FinishReason(); got != "stop" {
		t.Fatalf("finish_reason=%q want stop", got)
	}
}

// 非空 tool_calls 帧要累计（流式增量可能分多帧到达）。
func TestChatStatsToolCallsCounted(t *testing.T) {
	var s chatStatsReader
	s.parseSSELine(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"a"}]},"finish_reason":null}]}`)
	s.parseSSELine(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{}"}}]},"finish_reason":null}]}`)
	s.parseSSELine(`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`)
	if n := s.ToolCalls(); n != 2 {
		t.Fatalf("tool_calls 累计=%d want 2", n)
	}
	if got := s.FinishReason(); got != "tool_calls" {
		t.Fatalf("finish_reason=%q want tool_calls", got)
	}
}

// usage 与 choices 出现在同一个 chunk 时两者都要采到（解析重构不丢 usage）。
func TestChatStatsUsageAndChoicesSameChunk(t *testing.T) {
	var s chatStatsReader
	s.parseSSELine(`data: {"choices":[{"delta":{"content":"hi"},"finish_reason":"stop"}],"usage":{"completion_tokens":7,"total_tokens":9}}`)
	if n, ok := s.Tokens(); !ok || n != 7 {
		t.Fatalf("completion_tokens=%d ok=%v want 7/true", n, ok)
	}
	if got := s.FinishReason(); got != "stop" {
		t.Fatalf("finish_reason=%q want stop", got)
	}
}

// 错误帧仍然要被识别（重构保留了 error 检测）。
func TestChatStatsErrorFrameStillDetected(t *testing.T) {
	var s chatStatsReader
	s.parseSSELine(`data: {"error":{"message":"boom"}}`)
	if !s.SawErrorFrame() {
		t.Fatal("error 帧未识别")
	}
}
