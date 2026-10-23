package scheduler

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/linguo2625469/workbuddy2api-panel/internal/auth"
	"github.com/linguo2625469/workbuddy2api-panel/internal/pool"
	"github.com/linguo2625469/workbuddy2api-panel/internal/upstream"
)

// TestBalance401RefreshesAndRetries 余额周期任务遇到网关层 401（APISIX HTML，
// 无业务信封）时必须先强制 refresh 再用新 token 重试本号，而不是每 5 分钟
// 无限空刷 401（issue #5 的日志形态：balance 每轮 4 个号全 401，永远不恢复）。
func TestBalance401RefreshesAndRetries(t *testing.T) {
	var userCalls, refreshCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/v2/plugin/auth/token/refresh"):
			refreshCalls++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":0,"data":{"accessToken":"at-new","refreshToken":"rt-new","expiresIn":3600}}`))
		case strings.Contains(r.URL.Path, "get-user-resource"):
			userCalls++
			if r.Header.Get("Authorization") == "Bearer at-new" {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"code":0,"data":{"Response":{"Data":{"Accounts":[{"CycleCapacitySize":1000,"CycleCapacityRemain":900,"CycleCapacityUsed":100}]}}}}`))
				return
			}
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`<html><h1>401 Authorization Required</h1></html>`))
		default:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":0,"data":{}}`))
		}
	}))
	t.Cleanup(srv.Close)

	a := &auth.Auth{
		UID: "u1", AccessToken: "at-old", RefreshToken: "rt-old",
		ExpiresAt: 9999999999, FilePath: filepath.Join(t.TempDir(), "workbuddy-u1.json"),
	}
	p := pool.New("")
	p.Add(a)
	up := &upstream.Client{HTTP: srv.Client(), ChatBaseCN: srv.URL, BillingBaseCN: srv.URL, WebBaseCN: srv.URL}
	s := New(Config{Pool: p, Upstream: up})

	s.RunBalanceRefreshNow()

	if refreshCalls != 1 {
		t.Errorf("refresh 次数=%d want 1", refreshCalls)
	}
	if userCalls != 2 {
		t.Errorf("余额查询次数=%d want 2（401 + 刷新后重试）", userCalls)
	}
	st, ok := p.Status("u1")
	if !ok {
		t.Fatal("账号不在池中")
	}
	if st.Disabled {
		t.Errorf("refresh 成功后账号不应被禁用: %+v", st)
	}
	if a.AccessToken != "at-new" {
		t.Errorf("accessToken=%q want at-new", a.AccessToken)
	}
}
