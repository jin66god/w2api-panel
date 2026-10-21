package scheduler

import (
	"sort"
	"testing"
	"time"
)

// issue #45：偏移必须当天确定性——同一天重启后计划不变，否则重启即重排。
func TestSpreadPlanDeterministic(t *testing.T) {
	s := &Scheduler{cfg: Config{AccountSpreadMinutes: 60, AccountSpreadMinGapSeconds: 60}}
	uids := []string{"a", "b", "c", "d", "e"}
	p1 := s.planSpread(taskCheckin, uids)
	p2 := s.planSpread(taskCheckin, uids)
	if p1 == nil || p2 == nil {
		t.Fatal("窗口开启时不应返回 nil")
	}
	for _, u := range uids {
		if p1.offsets[u] != p2.offsets[u] {
			t.Fatalf("同一天两次计划不一致: %s %v vs %v", u, p1.offsets[u], p2.offsets[u])
		}
		if p1.offsets[u] < 0 || p1.offsets[u] > 60*time.Minute {
			t.Fatalf("偏移越界: %s %v", u, p1.offsets[u])
		}
	}
	// 最小间隔：排序后相邻不得低于 minGap（随机 + 冲突推开，不是等距）。
	offs := make([]time.Duration, 0, len(uids))
	for _, u := range uids {
		offs = append(offs, p1.offsets[u])
	}
	sort.Slice(offs, func(i, j int) bool { return offs[i] < offs[j] })
	for i := 1; i < len(offs); i++ {
		if offs[i]-offs[i-1] < 60*time.Second {
			t.Fatalf("相邻间隔小于下限: %v -> %v", offs[i-1], offs[i])
		}
	}
}

// 不同任务用不同盐：签到与旅行的偏移不应完全重合（否则同一账号两类任务
// 仍在同一时刻连打）。
func TestSpreadPlanKindSeparated(t *testing.T) {
	s := &Scheduler{cfg: Config{AccountSpreadMinutes: 60}}
	uids := []string{"a"}
	c := s.planSpread(taskCheckin, uids).offsets["a"]
	tr := s.planSpread(taskTravel, uids).offsets["a"]
	if c == tr {
		t.Fatalf("不同任务的偏移完全重合: %v", c)
	}
}

// 窗口 0 = 关闭，保持整点立即执行。
func TestSpreadPlanDisabled(t *testing.T) {
	s := &Scheduler{}
	if p := s.planSpread(taskCheckin, []string{"a"}); p != nil {
		t.Fatalf("窗口 0 应返回 nil: %+v", p)
	}
	var nilPlan *spreadPlan
	nilPlan.wait("a") // 不应 panic / 阻塞
}
