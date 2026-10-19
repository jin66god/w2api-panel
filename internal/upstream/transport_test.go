package upstream

import (
	"net/http"
	"testing"
)

// newTransport 必须认 HTTP(S)_PROXY 环境变量（#223）。自建 http.Transport 时
// Proxy 字段默认是 nil = 完全不读环境变量；漏掉这一行，容器里配了代理也不生效。
func TestNewTransportUsesEnvironmentProxy(t *testing.T) {
	tr := newTransport()
	if tr.Proxy == nil {
		t.Fatal("newTransport().Proxy 为 nil：环境变量代理不会生效")
	}
	req, err := http.NewRequest(http.MethodGet, "http://example.com/", nil)
	if err != nil {
		t.Fatal(err)
	}
	// 不关心是否有代理可返回，只要求 Proxy 函数可调用且不报错（无环境变量时返回 nil, nil）。
	if _, err := tr.Proxy(req); err != nil {
		t.Fatalf("Proxy(req) 报错：%v", err)
	}
}
