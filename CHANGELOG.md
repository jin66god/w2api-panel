# Changelog

本项目的版本变更记录。更早版本的发布说明见
[GitHub Releases](https://github.com/jin66god/workbuddy2api-panel/releases)。

## v1.17.3（2026-10-22）

### 修复

- **出站标识整体全面对齐官方 UUID 形状**：养号/活动上报链路里发往上游的
  `conversationId`、`requestId`、`traceId`、`rootRequestId` 与自动化任务名此前
  带 `wb2api-` 前缀或「前缀 + 毫秒时间戳」形状，属于可被上游风控按固定串或
  ID 形状批量筛选的第三方特征。现在统一由 `internal/upstream/official_ids.go`
  生成官方形状标识：
  - `conversationId` / `requestId` → UUID 形状（与前端口 `randomUUID` 一致）
  - `traceId` → 32 位十六进制
  - 定时任务名 → 官方产品名，不再携带中转标识
- **桌面端 `X-Conversation-ID` / `X-Request-ID`**：去掉 `wb2api-conv-<纳秒>`
  前缀与纯纳秒数字，改为官方形状，桌面专家对话请求不再自报家门。
- **命中范围**（全部出站，均已替换）：`upstream/desktop.go`、`upstream/school.go`、
  `upstream/blackcat.go`、`panel/autotask.go`（14 处）、`scheduler/scheduler.go`、
  `scheduler/travel.go`。
- 明确保留**不出站**的同名标识：`chatcmpl-wb2api`（下游 SSE id）、`msg_wb2api_`、
  Redis 前缀、哈希盐、临时文件名与测试数据，避免误改影响协议兼容。

### 验证

- `go build ./...` 通过；`go test ./internal/upstream ./internal/scheduler ./internal/panel` 全绿。
- 出站 grep 复核：`internal/`、`cmd/` 内除注释与 import 路径外无 `wb2api` 出站字符串。
