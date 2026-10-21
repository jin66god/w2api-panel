package upstream

import "testing"

// issue #46：14003 必须先于通用 429 兜底分类。按账号级限流处置会给该号打 60s，
// 三个号各命中一次就叠成整分钟黑障；按 ErrClient 处置则会无冷却连撞几十次。
func TestClassifyModelBusy(t *testing.T) {
	bodies := []string{
		`{"code":14003,"msg":"Model busy. Please switch models or retry later"}`,
		`{"code":"14003","msg":"Model busy. Please switch models or retry later"}`,
		`{"error":{"code":14003,"message":"Model busy. Please switch models or retry later"}}`,
		`Model busy. Please switch models or retry later`,
	}
	for _, status := range []int{200, 400, 429, 500, 503} {
		for _, b := range bodies {
			if got := Classify(status, b); got != ErrModelBusy {
				t.Errorf("status=%d body=%.32s => %v want ErrModelBusy", status, b, got)
			}
		}
	}
	// 反例：普通限流仍必须是 ErrSoftRate，不被 model busy 文案吞掉。
	if got := Classify(429, `{"code":6004,"msg":"rate limit exceeded"}`); got != ErrSoftRate {
		t.Errorf("普通限流误分类: %v", got)
	}
}
