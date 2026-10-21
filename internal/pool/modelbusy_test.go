package pool

import (
	"testing"
	"time"

	"github.com/linguo2625469/workbuddy2api-panel/internal/auth"
)

// issue #46：14003 只停「账号 × 模型」，账号级 until 必须保持零值——
// 否则三个号各命中一次就叠成整分钟「整池不可用」的黑障。
func TestCooldownModelBusyOnlyStopsThatModel(t *testing.T) {
	p := New("")
	p.Add(&auth.Auth{UID: "u1"})
	p.CooldownModelBusy("u1", "deepseek-v4.1-flash", 10*time.Second, "14003 model busy")

	now := time.Now()
	p.mu.Lock()
	e := p.byUID["u1"]
	accountCooled := !e.until.IsZero() && e.until.After(now)
	mc := e.modelCooldowns["deepseek-v4.1-flash"]
	p.mu.Unlock()

	if accountCooled {
		t.Fatalf("14003 不应写账号级 until（会把单模型繁忙放大成整号不可用）: until=%v", e.until)
	}
	if mc.Until.IsZero() || !mc.Until.After(now) {
		t.Fatalf("模型级冷却未写入: %+v", mc)
	}
	if e.healthyForModel(now, "deepseek-v4.1-flash") {
		t.Fatal("同一模型在 10s 窗口内应被避让")
	}
	if !e.healthyForModel(now, "glm-5.3") {
		t.Fatal("其它模型必须仍可选（账号整体健康）")
	}
	if !e.healthyForModel(now.Add(11*time.Second), "deepseek-v4.1-flash") {
		t.Fatal("10s 后模型应自动恢复")
	}
}

// 已有更晚的真实模型冷却时不能被 10s 覆盖（否则会把长冷却缩短成抖动窗口）。
func TestCooldownModelBusyDoesNotShortenLongerCooldown(t *testing.T) {
	p := New("")
	p.Add(&auth.Auth{UID: "u1"})
	long := time.Now().Add(2 * time.Hour)
	p.mu.Lock()
	p.byUID["u1"].modelCooldowns = map[string]modelCooldown{
		"deepseek-v4.1-flash": {Until: long, ResetAt: long},
	}
	p.mu.Unlock()

	p.CooldownModelBusy("u1", "deepseek-v4.1-flash", time.Second, "14003 model busy")
	p.mu.Lock()
	got := p.byUID["u1"].modelCooldowns["deepseek-v4.1-flash"].Until
	p.mu.Unlock()
	if !got.Equal(long) {
		t.Fatalf("更长的模型冷却被缩短: got %v want %v", got, long)
	}
}
