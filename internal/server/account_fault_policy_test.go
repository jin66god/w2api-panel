package server

import (
	"testing"
	"time"

	"github.com/linguo2625469/workbuddy2api-panel/internal/auth"
	"github.com/linguo2625469/workbuddy2api-panel/internal/pool"
	"github.com/linguo2625469/workbuddy2api-panel/internal/upstream"
)

// issue #197 / PR #198 回归：上游对「渠道校验失败」返回 400 + code 11-128 /
// msg "request illegal"，其 displayTips 明示这是可重试的请求级拦截，不是账号
// 封禁。历史实现在 ErrAccountFault 分支直接 Disable（永久踢出池子、需人工解冻），
// 把仍可用的账号误杀。此测试钉住：命中该文案时只做账号级软冷却（可自动恢复、
// 可轮换），绝不置 Disabled。
func TestApplyErrorPolicyRequestIllegalCooldownsNotDisables(t *testing.T) {
	p := pool.New("")
	p.Add(&auth.Auth{UID: "u1"})
	h := NewHandler(Config{Pool: p, SoftCooldown: 600 * time.Second})

	body := `{"code":"11-128","msg":"request illegal","displayMsg":{"zh":"请求被拦截，请重新发送"},"displayTips":{"zh":"渠道校验失败，可能不是从官方客户端发出的，请用官方客户端重新发送"}}`
	h.applyErrorPolicy("u1", upstream.ErrAccountFault, body, "", nil)

	st, ok := p.Status("u1")
	if !ok {
		t.Fatal("账号不在池中")
	}
	if st.Disabled {
		t.Fatalf("request illegal 不应永久禁用账号（issue #197 回归）: %+v", st)
	}
	if !st.Cooling {
		t.Fatalf("request illegal 应进入软冷却并轮换: %+v", st)
	}
	if st.CoolKind != "soft_rate" {
		t.Fatalf("cool_kind=%q, want soft_rate", st.CoolKind)
	}
	if st.CoolRemaining <= 0 {
		t.Fatalf("cool_remaining_sec=%d, want > 0", st.CoolRemaining)
	}
}

// 同一分支的 14017（trial not activated）仍走软冷却——补完 register 后自愈，
// 禁用会让用户补完后仍无法使用。此测试防止后续改动把两条子分支合并成 Disable。
func TestApplyErrorPolicyTrialNotActivatedStaysSoft(t *testing.T) {
	p := pool.New("")
	p.Add(&auth.Auth{UID: "u1"})
	h := NewHandler(Config{Pool: p, SoftCooldown: 600 * time.Second})

	h.applyErrorPolicy("u1", upstream.ErrAccountFault, `{"code":14017,"msg":"trial not activated"}`, "", nil)

	st, _ := p.Status("u1")
	if st.Disabled {
		t.Fatalf("14017 不应永久禁用账号: %+v", st)
	}
	if !st.Cooling || st.CoolKind != "soft_rate" {
		t.Fatalf("14017 应为 soft_rate 冷却: %+v", st)
	}
}
