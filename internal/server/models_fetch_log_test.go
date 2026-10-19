package server

import (
	"bytes"
	"log"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/linguo2625469/workbuddy2api-panel/internal/auth"
)

// CN 模型目录拉取失败必须留下一行 WARN（此前静默，排障无线索），且负缓存期内
// 的后续请求不重复记录。
func TestModelsFetchFailureLogsOncePerCooldown(t *testing.T) {
	dynamicModelsCache.Lock()
	dynamicModelsCache.ids = nil
	dynamicModelsCache.fetched = time.Time{}
	dynamicModelsCache.lastFail = time.Time{}
	dynamicModelsCache.Unlock()

	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(prev) })

	up := newFakeUpstream(t, func(authz string) (int, string, bool) {
		return 500, `boom`, false
	})
	p := testPoolWith(&auth.Auth{UID: "u1", AccessToken: "at1", ExpiresAt: 9999999999})
	h := NewHandler(Config{Pool: p, Upstream: up})

	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", "/v1/models", nil))
		if rec.Code != 200 {
			t.Fatalf("req %d: code=%d", i, rec.Code)
		}
	}
	const marker = "/v1/models: CN 模型目录拉取失败"
	if got := strings.Count(buf.String(), marker); got != 1 {
		t.Fatalf("want 1 failure log line, got %d; log:\n%s", got, buf.String())
	}
	if !strings.Contains(buf.String(), "status 500") {
		t.Errorf("failure log should carry upstream error; log:\n%s", buf.String())
	}
}
