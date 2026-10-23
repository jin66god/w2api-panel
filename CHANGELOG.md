# Changelog

本项目的版本变更记录。更早版本的发布说明见
[GitHub Releases](https://github.com/jin66god/workbuddy2api-panel/releases)。

## v1.17.7（2026-10-23）

### 修复

- **401 网关拒绝后的自愈**：上游网关（APISIX）在 token 被提前吊销时返回
  `401 Authorization Required` HTML，没有业务信封。此前这类响应归入通用 4xx：
  对话请求只换号、余额周期任务只打日志，既不会刷新 token，也不会标记账号失效，
  表现为「全部账号 401、只能人工重登」，且余额任务每 5 分钟无限空刷。
  现在单独分类为 `token_rejected`：对话与余额刷新各自**强制刷新一次**并用新
  token 重试同一账号；刷新失败才按连续 3 次会话失效软着陆禁用，单次 401 不杀号。
  搜索端点的 401 语义不唯一（可能是该号未开通搜索），只换号不累计会话失效。
- **企业切换会话不再阻断成长任务**：`token_source=enterprise_switch` 只记录本次
  授权走了企业切换流程，不代表账号处于企业上下文。成长任务门控改为只看
  `enterpriseId` 与静态 API Key，登录日志降为提示。修复微信扫码登录的个人号
  「成长任务无法执行」的问题。

## v1.17.6（2026-10-22）

### 安全

- 升级到 Go 1.27.3，修复 **CVE-2026-111943**（`crypto/x509` 对 8 个私钥外泄的
  eMudhra / emSign 中间 CA 取消信任）。验证时会拒绝包含这些中间证书的证书链，
  防止中间人攻击。`go.mod` 的 `go` 指令改为 `1.27.3`，Docker 构建镜像
  改为 `golang:1.27.3-alpine`，CI 通过 `go-version-file: go.mod` 自动使用 1.27.3。
  该过滤为硬编码、不可关闭，须重新编译才能生效。

## v1.17.5（2026-10-22）

### 修复

- 镜像构建失败：uTLS 依赖要求 Go ≥ 1.24，Dockerfile 构建镜像由 `golang:1.22-alpine`
  升级到 `golang:1.24-alpine`，修复 `go mod download` 退出码 1 导致的双架构镜像构建失败。

## v1.17.4（2026-10-22）

### 新增

- **出站 TLS 指纹对齐官方客户端**：18 个 cipher 顺序、12 个扩展顺序、9 项签名算法
  全部按真机抓包复刻，直连 HTTPS 走 uTLS 自定义 ClientHello。
  实测 `JA3=1a3153f314dc13e133dc71d113a81b16`、`JA4=t13d1812h1_5d04281c6031_ef7df7f74e48`，
  与官方业务 API 逐位一致；`WB2_TLS_FINGERPRINT=0` 可回退标准库。

### 变更

- 镜像仅在打 tag 时构建，日常合并到 main 不再触发双架构构建。
- Release 日志只保留面向用户的更新说明。

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
