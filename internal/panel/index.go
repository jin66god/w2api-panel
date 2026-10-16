// index.go 面板静态资源与安全响应头。
//
// 资源经 go:embed 打进二进制（随服务部署，无外部构建步骤）：
//   - index.html  页面骨架
//   - app.js      全部前端逻辑（独立文件而非内联，为了启用无需 unsafe-inline 的严格 CSP）
//   - mobile.css  手机端（≤760px）适配样式（本仓库自有，上游不维护 → 同步上游不冲突）
//   - mobile.js   手机端联动逻辑（抽屉开合、宽表卡片化字段名），须在 app.js 之后加载
//
// 安全头对"面板页面与全部 /panel/api/* 响应"统一生效：CSP 限制脚本只能来自本服务，
// 禁止被 iframe 嵌套（防点击劫持），禁 MIME 嗅探，并声明不泄露 Referer 出去。
package panel

import (
	"bytes"
	_ "embed"
	"net/http"
)

//go:embed index.html
var indexHTML []byte

//go:embed app.js
var appJS []byte

//go:embed mobile.css
var mobileCSS []byte

//go:embed mobile.js
var mobileJS []byte

// csp 内容安全策略（严格版，无需 unsafe-inline）：
//   - default-src 'none'        默认全禁，逐个开口
//   - script-src 'self'         只跑同源脚本（app.js / mobile.js）；页面无内联事件处理器/内联脚本
//   - style-src 'self' 'unsafe-inline'
//     style 的内联是设计取舍：页面有少量 style="..." 属性（进度条宽度、表格列宽），
//     允许内联样式不会导致脚本执行；仍禁止外部样式域与 @import 外链。
//   - connect-src 'self'        前端 fetch 只能打本服务
//   - img-src 'self' data:      图标/内联图
//   - form-action 'none'        页面无表单提交目标（配置页是 JS 提交）
//   - frame-ancestors 'none'    禁止被任何站点 iframe 嵌套（点击劫持）
//   - base-uri 'none'          禁止注入 <base> 改写相对路径
const csp = "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; " +
	"connect-src 'self'; img-src 'self' data:; form-action 'none'; " +
	"frame-ancestors 'none'; base-uri 'none'"

// setSecurityHeaders 写入面板统一安全响应头（页面与 API 都要，API 也含 JSON 数据）。
func setSecurityHeaders(w http.ResponseWriter) {
	// 前端资源随二进制发布：必须禁缓存（no-cache = 每次使用前回源校验），否则升级后
	// 浏览器可能仍用旧 app.js —— 新页面配旧脚本，新增按钮没有事件绑定，表现为
	// 「点『收起』完全没反应」（真实反馈）。API 同样禁缓存，避免中间层缓存账号数据。
	w.Header().Set("Cache-Control", "no-cache, must-revalidate")
	w.Header().Set("Content-Security-Policy", csp)
	w.Header().Set("X-Content-Type-Options", "nosniff") // 禁 MIME 嗅探
	w.Header().Set("X-Frame-Options", "DENY")           // 老浏览器兜底（CSP frame-ancestors 的等价项）
	w.Header().Set("Referrer-Policy", "no-referrer")    // 不外泄面板地址给外部站点
	w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
}

// index 输出面板页面（静态无秘密；数据接口 /panel/api/* 才走鉴权）。
//
// 页面里的静态资源引用带版本查询串（?v=<版本>）：面板常挂在 Cloudflare 等 CDN 后，
// .js / .css 会被边缘缓存数小时（CF 对无缓存头的静态扩展名默认 4h），而**升级二进制
// 不会让 CDN 回源** —— 结果是「新页面 + 旧 app.js」，新增按钮没有事件绑定，用户侧表现
// 为「点了完全没反应」（真实反馈：1.13.4 的「收起」按钮，源站已升级、CF 仍 HIT 旧 app.js）。
// URL 带版本后每次发版 URL 都变，CDN 与浏览器缓存自动失效，无需人工 purge。
func (p *Panel) index(w http.ResponseWriter, r *http.Request) {
	setSecurityHeaders(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(versionedIndex(p.cfg.Version))
}

// versionedIndex 给 index.html 里的 app.js / mobile.js / mobile.css 引用加 ?v=<版本>。
// 版本为空（未注入）时原样返回。只对 3 个固定资源名做字节替换（微秒级），不做缓存。
func versionedIndex(version string) []byte {
	if version == "" {
		return indexHTML
	}
	out := indexHTML
	for _, asset := range []string{"app.js", "mobile.js", "mobile.css"} {
		out = bytes.ReplaceAll(out, []byte(`"`+asset+`"`), []byte(`"`+asset+`?v=`+version+`"`))
	}
	return out
}

// appScript 输出前端逻辑（同源脚本，供 CSP script-src 'self' 加载）。
func (p *Panel) appScript(w http.ResponseWriter, r *http.Request) {
	setSecurityHeaders(w)
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(appJS)
}

// mobileStyle 输出手机端适配样式（同源样式表，CSP style-src 'self' 加载）。
// 单独成文而非写进 index.html：上游频繁改 index.html，独立文件可避免同步上游时反复冲突。
func (p *Panel) mobileStyle(w http.ResponseWriter, r *http.Request) {
	setSecurityHeaders(w)
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(mobileCSS)
}

// mobileScript 输出手机端联动逻辑（同源脚本，须在 app.js 之后加载）。
func (p *Panel) mobileScript(w http.ResponseWriter, r *http.Request) {
	setSecurityHeaders(w)
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(mobileJS)
}
