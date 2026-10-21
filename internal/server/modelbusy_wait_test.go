package server

import (
	"testing"
	"time"

	"github.com/linguo2625469/workbuddy2api-panel/internal/pool"
)

// issue #46：只有「全池 14003 + 最早解封在 10s 内 + 本请求没等过」才值得
// 在请求内等一次。其余情况必须立刻报错，不能拖住客户端。
func TestModelBusyWaitFor(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name          string
		st            pool.ModelBlockStatus
		alreadyWaited bool
		want          bool
	}{
		{"14003 5s 内等待", pool.ModelBlockStatus{Blocked: true, Reason: modelBusyReason, Until: now.Add(5 * time.Second)}, false, true},
		{"边界 10s", pool.ModelBlockStatus{Blocked: true, Reason: modelBusyReason, Until: now.Add(modelBusyWaitMax)}, false, true},
		{"超过 10s 不等", pool.ModelBlockStatus{Blocked: true, Reason: modelBusyReason, Until: now.Add(30 * time.Second)}, false, false},
		{"已等过一次不再等", pool.ModelBlockStatus{Blocked: true, Reason: modelBusyReason, Until: now.Add(time.Second)}, true, false},
		{"11102 换号必败，不等", pool.ModelBlockStatus{Blocked: true, Reason: "11102 model not found", Until: now.Add(time.Second)}, false, false},
		{"6004 长冷却不等", pool.ModelBlockStatus{Blocked: true, Reason: "6004 model rate limit", Until: now.Add(time.Second)}, false, false},
		{"非全池阻塞不等", pool.ModelBlockStatus{Blocked: false, Reason: modelBusyReason, Until: now.Add(time.Second)}, false, false},
		{"无解封时间不等", pool.ModelBlockStatus{Blocked: true, Reason: modelBusyReason}, false, false},
	}
	for _, c := range cases {
		wait, ok := modelBusyWaitFor(c.st, c.alreadyWaited)
		if ok != c.want {
			t.Errorf("%s: ok=%v want %v", c.name, ok, c.want)
			continue
		}
		if ok && (wait <= 0 || wait > modelBusyWaitMax) {
			t.Errorf("%s: wait=%v 超出边界", c.name, wait)
		}
	}
}
