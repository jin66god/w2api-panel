package upstream

import (
	"crypto/rand"
	"encoding/hex"
)

// 上游风控可以按键名前缀/固定标识筛选第三方中转流量。所有**出站**到官方的
// conversationId / requestId / traceId / 自动化名称一律使用官方客户端形状，
// 禁止携带本项目名称或品牌词（wb2api 等）。
//
// 形状依据：官方客户端同类字段为 UUID 形状（前端 randomUUID）或 32 位十六进制，
// 见 streak.go clientToken 的实测口径。
func OfficialConversationID() string { return clientToken() }

// OfficialRequestID 与 conversationId 同形但独立取值（同一会话内多轮 requestId 不同）。
func OfficialRequestID() string { return clientToken() }

// OfficialTraceID 32 位十六进制，对齐服务端 traceId 形状。
func OfficialTraceID() string { return hexToken(32) }

// OfficialAutomationName 定时任务名称使用官方产品名，不带任何中转标识。
func OfficialAutomationName() string { return "WorkBuddy 自动化" }

func hexToken(n int) string {
	if n <= 0 {
		n = 32
	}
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)[:n]
}
