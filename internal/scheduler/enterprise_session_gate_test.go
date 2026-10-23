package scheduler

import (
	"encoding/base64"
	"testing"
	"time"

	"github.com/linguo2625469/workbuddy2api-panel/internal/auth"
	"github.com/linguo2625469/workbuddy2api-panel/internal/pool"
	"github.com/linguo2625469/workbuddy2api-panel/internal/upstream"
)

// sessionTestScheduler 与 enterpriseTestScheduler 同构：switched=true 时账号的
// AccessToken 为带 token_source=enterprise_switch 的 JWT（企业**切换会话**，
// enterpriseId 为空——网关无法凭 enterpriseId 识别的形态）；false 为个人号对照组。
func sessionTestScheduler(t *testing.T, switched bool) (*Scheduler, *recorderUpstream) {
	t.Helper()
	r := &recorderUpstream{}
	srv := r.server()
	t.Cleanup(srv.Close)

	token := "at"
	if switched {
		payload := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"u1","token_source":"enterprise_switch"}`))
		token = "hdr." + payload + ".sig"
	}
	a := &auth.Auth{UID: "u1", AccessToken: token, RefreshToken: "rt", ExpiresAt: 9999999999}
	p := pool.New("")
	p.Add(a)

	up := &upstream.Client{
		HTTP:          srv.Client(),
		ChatBaseCN:    srv.URL,
		BillingBaseCN: srv.URL,
		WebBaseCN:     srv.URL,
	}
	s := New(Config{Pool: p, Upstream: up})
	return s, r
}

// TestEnterpriseSessionNotGated 企业切换会话（token_source=enterprise_switch）不再
// 触发任何任务门控（issue #4）：该声明只记录本次授权走了企业切换流程，不代表账号
// 处于企业上下文。实测（misakano7545 #240）enterpriseId 为空的个人号成长任务照常
// claimed、行为事件正常计分；本库早期 #162 观测到的静默丢弃属上游临时状态，次日
// 即自愈，不能作为稳定判据。五类任务对企业切换会话必须与个人号完全一致地发起。
func TestEnterpriseSessionNotGated(t *testing.T) {
	// 与 TestEnterpriseGateSkipsGrowthTasks 同款：把夜猫子窗口钉在窗口内。
	restoreClock := nowForNightWindow
	nowForNightWindow = func() time.Time { return time.Date(2026, 10, 12, 23, 30, 0, 0, time.Local) }
	t.Cleanup(func() { nowForNightWindow = restoreClock })

	cases := []struct {
		name string
		run  func(*Scheduler)
		// endpoint 是该任务在个人号上必然发起的上游路径。
		endpoint string
	}{
		{"签到", func(s *Scheduler) { s.RunCheckinNow() }, "/daily-checkin"},
		{"活跃上报", func(s *Scheduler) { s.RunActivityNow() }, "/v2/report"},
		{"猫猫旅行", func(s *Scheduler) { s.RunTravelNow() }, "/activity/growth/buddy/info"},
		{"夜猫子", func(s *Scheduler) { s.RunBlackcatNow() }, "/v2/activity/growth/tasks"},
		{"连登管家", func(s *Scheduler) { s.RunStreakBonusNow() }, "/activity/growth/streak"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sSess, rSess := sessionTestScheduler(t, true)
			c.run(sSess)
			if !rSess.has(c.endpoint) {
				t.Errorf("企业切换会话应照常发起 %s（issue #4：token_source 不参与门控），实际请求=%v", c.endpoint, rSess.snapshot())
			}

			// 个人号对照组：证明该断言不是「构造有误所以都发」。
			sPer, rPer := sessionTestScheduler(t, false)
			c.run(sPer)
			if !rPer.has(c.endpoint) {
				t.Errorf("个人号对照应发起 %s，实际请求=%v", c.endpoint, rPer.snapshot())
			}
		})
	}
}

// TestEnterpriseSessionKeepsKeepalive 切换会话照常保活（token 会过期，会话
// 形态不影响选号/刷新能力）。
func TestEnterpriseSessionKeepsKeepalive(t *testing.T) {
	s, r := sessionTestScheduler(t, true)
	s.RunKeepaliveNow()
	if !r.has("/token/refresh") {
		t.Errorf("企业切换会话必须照常保活，实际请求=%v", r.snapshot())
	}
}
