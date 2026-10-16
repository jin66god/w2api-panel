// cache_key.go 注入上游 prompt_cache_key 字段（P0 费用优化）。
//
// 逆向实测（buddy-adapter.ts:706-714）：上游服务端支持 prompt_cache_key，
// 同一段 8k token 前缀：
//   - 不带 → prompt_cache_hit_tokens=0, credit≈0.34
//   - 带   → prompt_cache_hit_tokens=7808, credit≈0.02（费用降 ~17×）
//
// 网关在此为每个出站 chat 请求注入一个稳定、按账号隔离的 cache key，
// 让同一客户端对同一账号的连续请求复用上游前缀缓存。
package upstream

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

// InjectPromptCacheKey 在已改写的出站 body 上注入 prompt_cache_key 字段。
//
// 优先级：
//  1. body 已带 prompt_cache_key → 原值保留（客户端自知复用哪个键）
//  2. body 已带 conversation_id / conversationId → 用它做会话哈希源
//  3. 都没有 → 用入站 conversationID 参数（来自 X-Conversation-ID 头解析）
//
// 另外同步补齐 body 级 conversation_id（issue #169）：部分上游后端（hy4-preview-f /
// step-5-preview 等）按 body 的 conversation_id 分前缀缓存槽、**不认 prompt_cache_key**；
// body 缺该字段时同一账号上所有会话会塌进同一个槽互相驱逐，命中率被钉在客户端提示词的
// 公共静态头后翻转归零（实测 hy4 命中 63%、延续态整段未命中 36%，补齐后 90%+）。
// 客户端已带会话字段时绝不覆盖，优先级语义不变。
//
// 安全约束——按账号隔离：
//   - 生成键格式 `wb2a-<uid8>-<convHex>`
//   - uid8 是账号 UID 前 8 字符，跨账号绝不相同 → 跨账号缓存键绝不碰撞
//   - 跨账号复用同一 cache key 会让上游命中错账号的前缀缓存、泄露对方对话，故 uid 是硬隔离因子
//
// 入参 uid 为账号 UID（空则用 "-"，但仍会注入键；调用方应保证传真实 UID）。
// 入参 conversationID 为网关解析出的会话标识（body 里没有 conversation_id 时用它做哈希源）。
// 两源都空时 convHex 为定值（每次新会话不复用前缀，但仍保留账号隔离段），且不注入
// body 级 conversation_id（没有会话可表达，注空串只会多一个无意义字段）。
// body 不可解析时原样返回（与 prepareBody 语义一致：坏 body 不二次错误化）。
func InjectPromptCacheKey(body []byte, uid, conversationID string) []byte {
	if len(body) == 0 {
		return body
	}
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		return body
	}
	// 会话源（两个下游分支共用）：body 自带 > 入站参数。bodyHasConv 记录 body 是否本来
	// 就带会话字段——补齐 conversation_id 时只填空缺，绝不覆盖客户端原值。
	conv, bodyHasConv := conversationID, false
	if v := strField(obj, "conversation_id"); v != "" {
		conv, bodyHasConv = v, true
	} else if v := strField(obj, "conversationId"); v != "" {
		conv, bodyHasConv = v, true
	}
	changed := false
	// 优先级 1：客户端已显式带 key → 绝不覆盖（但它未必带了会话字段，仍走下面的补齐）。
	if existing, ok := obj["prompt_cache_key"].(string); !ok || existing == "" {
		obj["prompt_cache_key"] = buildCacheKey(uid, conv)
		changed = true
	}
	// 补齐 body 级 conversation_id（issue #169）：与 prompt_cache_key 的会话成分同源
	// （同一 conv 驱动），不会引入两个字段口径不一致。
	if conv != "" && !bodyHasConv {
		obj["conversation_id"] = conv
		changed = true
	}
	if !changed {
		// 客户端自带 key 且自带会话字段：原样返回，不做无谓的重新序列化。
		return body
	}
	out, err := json.Marshal(obj)
	if err != nil {
		return body
	}
	return out
}

// buildCacheKey 生成 `wb2a-<uid8>-<convHex>` 格式的稳定 cache key。
//
// uid8 提供账号隔离段；convHex = sha256(uid + conversationID)[:16] 的 hex 提供会话段
// （同账号同会话稳定、不同会话不同）。会话源为空时 convHex 仍由 uid 单独哈希，
// 保证跨账号绝不碰撞但同一空会话不复用（空会话 = 新会话语义）。
func buildCacheKey(uid, conversation string) string {
	uid8 := uid
	if len(uid8) > 8 {
		uid8 = uid8[:8]
	}
	if uid8 == "" {
		uid8 = "-"
	}
	sum := sha256.Sum256([]byte(uid + "|" + conversation))
	convHex := hex.EncodeToString(sum[:16])
	return "wb2a-" + uid8 + "-" + convHex
}

// strField 从 map 取 string 字段，非 string 或空串返回 ""。
func strField(obj map[string]any, key string) string {
	v, ok := obj[key].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(v)
}
