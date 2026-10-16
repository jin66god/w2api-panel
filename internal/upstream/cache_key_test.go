package upstream

import (
	"encoding/json"
	"strings"
	"testing"
)

// extractCacheKey 从改写后的 body 里取出 prompt_cache_key 字段值。
func extractCacheKey(t *testing.T, body []byte) string {
	t.Helper()
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	v, _ := obj["prompt_cache_key"].(string)
	return v
}

// TestInjectPromptCacheKey_PreservesExisting 覆盖任务用例 1：
// 入站 body 已带 prompt_cache_key → 原值保留，不被网关覆盖。
func TestInjectPromptCacheKey_PreservesExisting(t *testing.T) {
	// Arrange
	uid := "user-abc-123"
	conv := "conv-xyz"
	in := `{"model":"glm-5.2","messages":[],"prompt_cache_key":"client-set-key"}`

	// Act
	out := InjectPromptCacheKey([]byte(in), uid, conv)

	// Assert
	if got := extractCacheKey(t, out); got != "client-set-key" {
		t.Fatalf("existing prompt_cache_key overwritten: got=%q want client-set-key", got)
	}
}

// TestInjectPromptCacheKey_UsesConversationID 覆盖任务用例 2：
// body 无 prompt_cache_key 但有 conversation_id → 用它做会话哈希源。
// 格式 wb2a-<uid8>-<conversationHash>：conversation_id 不逐字出现，而是驱动哈希段
// （同 id 同 key、不同 id 不同 key）。同时仍带 uid8 隔离前缀。
func TestInjectPromptCacheKey_UsesConversationID(t *testing.T) {
	uid := "user-abc-123"
	in := `{"model":"glm-5.2","messages":[],"conversation_id":"conv-42"}`

	out := InjectPromptCacheKey([]byte(in), uid, "")
	got := extractCacheKey(t, out)
	if !strings.HasPrefix(got, "wb2a-") {
		t.Fatalf("expected wb2a- prefix, got=%q", got)
	}
	if !strings.Contains(got, "user-abc") {
		t.Fatalf("expected uid8 isolation prefix, got=%q", got)
	}
	// 同 conversation_id 两次出站 → 同 key（稳定）。
	out2 := InjectPromptCacheKey([]byte(in), uid, "")
	if got2 := extractCacheKey(t, out2); got2 != got {
		t.Fatalf("non-deterministic key for same conversation_id: %q vs %q", got, got2)
	}
	// 不同 conversation_id → 不同 key（哈希段区分）。
	inB := `{"model":"glm-5.2","messages":[],"conversation_id":"conv-99"}`
	gotB := extractCacheKey(t, InjectPromptCacheKey([]byte(inB), uid, ""))
	if gotB == got {
		t.Fatalf("different conversation_id produced same key: %q", got)
	}
}

// TestInjectPromptCacheKey_GeneratesStableKey 覆盖任务用例 3：
// body 两者都无 → 网关生成稳定键，两次同输入同账号得到同 key。
func TestInjectPromptCacheKey_GeneratesStableKey(t *testing.T) {
	uid := "user-abc-123"
	conv := "conv-stable"
	in := `{"model":"glm-5.2","messages":[]}`

	a := InjectPromptCacheKey([]byte(in), uid, conv)
	b := InjectPromptCacheKey([]byte(in), uid, conv)
	ka, kb := extractCacheKey(t, a), extractCacheKey(t, b)
	if ka != kb {
		t.Fatalf("non-deterministic key for same input+uid: a=%q b=%q", ka, kb)
	}
}

// TestInjectPromptCacheKey_AccountIsolation 覆盖任务用例 4：
// 不同账号 → key 不同（隔离验证）。
func TestInjectPromptCacheKey_AccountIsolation(t *testing.T) {
	conv := "conv-shared"
	in := `{"model":"glm-5.2","messages":[]}`

	ka := extractCacheKey(t, InjectPromptCacheKey([]byte(in), "user-aaa-111", conv))
	kb := extractCacheKey(t, InjectPromptCacheKey([]byte(in), "user-bbb-222", conv))
	if ka == kb {
		t.Fatalf("cross-account key collision: both=%q", ka)
	}
	if !strings.HasPrefix(ka, "wb2a-") || !strings.HasPrefix(kb, "wb2a-") {
		t.Fatalf("keys must carry wb2a- prefix: a=%q b=%q", ka, kb)
	}
}

// TestInjectPromptCacheKey_Format 覆盖任务用例 5：
// cache key 格式校验（含 uid8 前缀）。
func TestInjectPromptCacheKey_Format(t *testing.T) {
	uid := "1234567890abcdef"
	in := `{"model":"glm-5.2","messages":[]}`
	out := InjectPromptCacheKey([]byte(in), uid, "conv-fmt")
	got := extractCacheKey(t, out)
	if !strings.HasPrefix(got, "wb2a-") {
		t.Fatalf("expected wb2a- prefix, got=%q", got)
	}
	if !strings.Contains(got, "12345678") {
		t.Fatalf("expected uid8 (12345678) in key, got=%q", got)
	}
}

// TestInjectPromptCacheKey_EmptyConversation 覆盖任务用例 3 边界：
// 无 conversation_id 且无会话标识 → 生成键含 uid8 但 conversationHash 段为定值（不复用前缀）。
// 不应报错、不应空串（key 非空）。
func TestInjectPromptCacheKey_EmptyConversation(t *testing.T) {
	uid := "user-abc-123"
	in := `{"model":"glm-5.2","messages":[]}`
	out := InjectPromptCacheKey([]byte(in), uid, "")
	got := extractCacheKey(t, out)
	if got == "" {
		t.Fatalf("expected non-empty key when no conversation, got empty")
	}
	if !strings.HasPrefix(got, "wb2a-") {
		t.Fatalf("expected wb2a- prefix, got=%q", got)
	}
	if !strings.Contains(got, "user-abc") {
		t.Fatalf("expected uid8 in key, got=%q", got)
	}
}

// TestPrepareBodyOptNoCacheKeyInjection 覆盖任务用例 6：
// PrepareBodyOpt（sanitize=false 的旧入口）行为不变——不注入 cache key。
// 向后兼容：仅传 body 不给 cacheKey 上下文时，不得引入新字段。
func TestPrepareBodyOptNoCacheKeyInjection(t *testing.T) {
	in := `{"model":"glm-5.2","messages":[]}`
	out := PrepareBodyOpt([]byte(in), false)
	var obj map[string]any
	if err := json.Unmarshal(out, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, present := obj["prompt_cache_key"]; present {
		t.Fatalf("PrepareBodyOpt must NOT inject prompt_cache_key, but got: %v", obj["prompt_cache_key"])
	}
}

// extractConvID 从改写后的 body 里取出 conversation_id 字段值及其存在性。
func extractConvID(t *testing.T, body []byte) (string, bool) {
	t.Helper()
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	v, ok := obj["conversation_id"].(string)
	return v, ok
}

// TestInjectPromptCacheKey_InjectsBodyConversationID 覆盖 issue #169 主用例：
// 客户端不发 conversation_id（生产形态）→ 网关补齐 body 级 conversation_id，
// 让按 conversation_id 分前缀缓存槽的后端（hy4-preview-f / step-5-preview 等）
// 不再把同账号所有会话塌进一个槽互相驱逐（实测整段未命中 0% vs 补齐后 92%）。
func TestInjectPromptCacheKey_InjectsBodyConversationID(t *testing.T) {
	in := `{"model":"hy4-preview-f","messages":[]}`
	out := InjectPromptCacheKey([]byte(in), "user-abc-123", "conv-from-header")
	conv, ok := extractConvID(t, out)
	if !ok || conv != "conv-from-header" {
		t.Fatalf("body conversation_id 未补齐: got=%q present=%v", conv, ok)
	}
	if got := extractCacheKey(t, out); got == "" {
		t.Fatalf("prompt_cache_key 丢失")
	}
}

// TestInjectPromptCacheKey_KeepsClientConversationID 客户端已带会话字段时不得覆盖
// （优先级语义不变），且缓存键的会话成分必须与 body 的 conversation_id 同源。
func TestInjectPromptCacheKey_KeepsClientConversationID(t *testing.T) {
	in := `{"model":"hy4-preview-f","messages":[],"conversation_id":"client-conv"}`
	out := InjectPromptCacheKey([]byte(in), "user-abc-123", "conv-from-header")
	if conv, _ := extractConvID(t, out); conv != "client-conv" {
		t.Fatalf("客户端 conversation_id 被覆盖: %q", conv)
	}
	keyFromClient := extractCacheKey(t, out)
	keyFromHeader := extractCacheKey(t, InjectPromptCacheKey(
		[]byte(`{"model":"hy4-preview-f","messages":[]}`), "user-abc-123", "client-conv"))
	if keyFromClient != keyFromHeader {
		t.Fatalf("键与 body conversation_id 不同源: %q vs %q", keyFromClient, keyFromHeader)
	}
}

// TestInjectPromptCacheKey_KeepsCamelConversationID 驼峰 conversationId 同样算「已带会话字段」，
// 不再额外补 snake 字段（避免同一会话出现两个字段）。
func TestInjectPromptCacheKey_KeepsCamelConversationID(t *testing.T) {
	in := `{"model":"hy4-preview-f","messages":[],"conversationId":"camel-conv"}`
	out := InjectPromptCacheKey([]byte(in), "user-abc-123", "conv-from-header")
	if conv, ok := extractConvID(t, out); ok {
		t.Fatalf("客户端已带 conversationId 时不应再注入 conversation_id: %q", conv)
	}
}

// TestInjectPromptCacheKey_NoConversationSource 两源都空 → 不注入 conversation_id
// （没有会话可表达），但仍注入账号隔离的 prompt_cache_key。
func TestInjectPromptCacheKey_NoConversationSource(t *testing.T) {
	in := `{"model":"hy4-preview-f","messages":[]}`
	out := InjectPromptCacheKey([]byte(in), "user-abc-123", "")
	if conv, ok := extractConvID(t, out); ok {
		t.Fatalf("无会话源时不应注入 conversation_id: %q", conv)
	}
	if got := extractCacheKey(t, out); got == "" {
		t.Fatalf("无会话源时仍应注入 prompt_cache_key")
	}
}

// TestInjectPromptCacheKey_ClientKeyAlsoGetsConversationID 客户端自带 prompt_cache_key
// 但没带会话字段时：键原值保留，同时补齐 conversation_id——否则这类客户端在不认
// prompt_cache_key 的后端上仍会塌槽（issue #169 的另一半）。
func TestInjectPromptCacheKey_ClientKeyAlsoGetsConversationID(t *testing.T) {
	in := `{"model":"hy4-preview-f","messages":[],"prompt_cache_key":"client-set-key"}`
	out := InjectPromptCacheKey([]byte(in), "user-abc-123", "conv-from-header")
	if got := extractCacheKey(t, out); got != "client-set-key" {
		t.Fatalf("客户端 key 被覆盖: %q", got)
	}
	if conv, ok := extractConvID(t, out); !ok || conv != "conv-from-header" {
		t.Fatalf("应补齐 conversation_id: got=%q present=%v", conv, ok)
	}
}

// TestInjectPromptCacheKey_ClientKeyAndConvUntouched 客户端 key 与会话字段都齐时
// 原样返回（不做无谓的重新序列化，保持字节稳定）。
func TestInjectPromptCacheKey_ClientKeyAndConvUntouched(t *testing.T) {
	in := `{"model":"hy4-preview-f","messages":[],"prompt_cache_key":"k","conversation_id":"c"}`
	out := InjectPromptCacheKey([]byte(in), "user-abc-123", "conv-from-header")
	if string(out) != in {
		t.Fatalf("两字段齐全时不应改写 body:\n got=%s\nwant=%s", out, in)
	}
}
