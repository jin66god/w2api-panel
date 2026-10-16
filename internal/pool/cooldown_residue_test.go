package pool

import (
	"testing"
	"time"

	"github.com/linguo2625469/workbuddy2api-panel/internal/auth"
)

// issue #171 问题一回归：账号级软冷却（429 rate limit）到期后，until / cool_kind /
// reason 三个字段仍留在内存与 state.json 里（只有再次冷却、禁用或复活才会重写）。
// 修复要求三处口径一致——状态透出、落盘、恢复都只在冷却仍未过期时认这三个字段，
// 否则面板会把「可用」与一周前的 429 rate limit 并排展示，运维无法判断是历史残留
// 还是当前限流。

// 过期冷却残留不得出现在状态端点里。
func TestStatusHidesExpiredCooldownResidue(t *testing.T) {
	p := New("")
	p.Add(&auth.Auth{UID: "u1"})
	p.mu.Lock()
	e := p.byUID["u1"]
	e.reason = "429 rate limit"
	e.coolKind = CoolSoft
	e.until = time.Now().Add(-time.Hour) // 已到期的冷却残留
	p.mu.Unlock()

	st, ok := p.Status("u1")
	if !ok {
		t.Fatal("status(u1) 不存在")
	}
	if st.Cooling {
		t.Fatalf("过期冷却不应报 cooling: %+v", st)
	}
	if st.Reason != "" {
		t.Errorf("过期冷却的 reason 泄漏到状态端点: %q", st.Reason)
	}
	if !st.Until.IsZero() {
		t.Errorf("过期冷却的 until 泄漏到状态端点: %v", st.Until)
	}
}

// 生效中的冷却必须照常透出（防修过头：把真限流一起藏了）。
func TestStatusKeepsActiveCooldownReason(t *testing.T) {
	p := New("")
	p.Add(&auth.Auth{UID: "u1"})
	p.mu.Lock()
	e := p.byUID["u1"]
	e.reason = "429 rate limit"
	e.coolKind = CoolSoft
	e.until = time.Now().Add(time.Hour)
	p.mu.Unlock()

	st, ok := p.Status("u1")
	if !ok {
		t.Fatal("status(u1) 不存在")
	}
	if !st.Cooling || st.Reason != "429 rate limit" || st.Until.IsZero() {
		t.Fatalf("生效中的冷却应完整透出: %+v", st)
	}
}

// 落盘：过期冷却残留不写，生效中的冷却照写。
func TestPersistDropsExpiredCooldownResidue(t *testing.T) {
	p := New("")
	p.Add(&auth.Auth{UID: "u1"})
	now := time.Now()
	p.mu.Lock()
	e := p.byUID["u1"]
	e.reason = "429 rate limit"
	e.coolKind = CoolSoft
	e.until = now.Add(-time.Hour)
	expired := p.stateOverviewLocked().Accounts["u1"]
	e.until = now.Add(time.Hour)
	active := p.stateOverviewLocked().Accounts["u1"]
	p.mu.Unlock()

	if expired.Reason != "" || !expired.Until.IsZero() || expired.CoolKind != 0 {
		t.Errorf("过期冷却残留被写进 state.json: %+v", expired)
	}
	if active.Reason != "429 rate limit" || active.CoolKind != CoolSoft || !active.Until.After(now) {
		t.Errorf("生效中的冷却应完整落盘: %+v", active)
	}
}

// 禁用原因与冷却正交：disabled 号的 reason 必须继续落盘（否则重启后看不到为什么死）。
func TestPersistKeepsDisabledReason(t *testing.T) {
	p := New("")
	p.Add(&auth.Auth{UID: "u1"})
	p.Disable("u1", "12153 session dead")
	p.mu.Lock()
	sa := p.stateOverviewLocked().Accounts["u1"]
	p.mu.Unlock()

	if !sa.Disabled || sa.Reason != "12153 session dead" {
		t.Fatalf("禁用原因应照常落盘: %+v", sa)
	}
}

// 恢复：旧 state.json 里的过期冷却残留不复活；生效中的冷却与禁用原因照常恢复。
func TestRestoreDropsExpiredCooldownResidue(t *testing.T) {
	p := New("")
	now := time.Now()
	p.applyAccountsLocked(map[string]stateAccount{
		"u1": {Reason: "429 rate limit", CoolKind: CoolSoft, Until: now.Add(-time.Hour)},
		"u2": {Reason: "429 rate limit", CoolKind: CoolSoft, Until: now.Add(time.Hour)},
		"u3": {Disabled: true, Reason: "12153 session dead", CoolKind: CoolSoft, Until: now.Add(-time.Hour)},
	})

	p.mu.RLock()
	defer p.mu.RUnlock()
	if e := p.byUID["u1"]; e.reason != "" || !e.until.IsZero() || e.coolKind != 0 {
		t.Errorf("过期冷却残留被复活: reason=%q until=%v kind=%v", e.reason, e.until, e.coolKind)
	}
	if e := p.byUID["u2"]; e.reason != "429 rate limit" || !e.until.After(now) || e.coolKind != CoolSoft {
		t.Errorf("生效中的冷却应恢复: reason=%q until=%v kind=%v", e.reason, e.until, e.coolKind)
	}
	if e := p.byUID["u3"]; !e.disabled || e.reason != "12153 session dead" {
		t.Errorf("禁用原因应恢复: disabled=%v reason=%q", e.disabled, e.reason)
	}
}
