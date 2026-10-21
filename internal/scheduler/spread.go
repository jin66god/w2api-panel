package scheduler

import (
	"hash/fnv"
	"sort"
	"time"
)

// 账号级随机摊开（schedule.account_spread_minutes / account_spread_min_gap_seconds）。
//
// 背景：排程原先只做到任务级——同一时点所有账号按 Pool.List() 顺序、800ms 级
// 间隔连续执行，10 个账号在十秒内全部打完。若上游按「同一出口 IP + 固定时点 +
// 多账号同批动作」做关联，这是一个稳定的时间指纹。本机制只降低时间相关性，
// 不解决 IP / 设备指纹。
//
// 设计口径（与上游 issue #45 的结论一致）：
//   - 两个全局参数，不按任务分别配（上游风控差异是未知量，多配只会变成猜数字）；
//   - 默认 0/0 = 关闭，保持整点立即执行；手动入口始终立即执行；
//   - 偏移当天确定性：hash(uid|kind|日期) → [0, 窗口)，同一天重启不变；
//   - 不是等距：先随机，只有落入 minGap 冲突时才把后者推到前者之后。
const (
	// spreadHashSalt 变更时会让所有部署的偏移重排；只在算法本身改变时递增。
	spreadHashSalt = "wb2api-spread-v1"
)

// spreadSettings 返回窗口与最小间隔。窗口 <= 0 表示关闭。
func (s *Scheduler) spreadSettings() (window, minGap time.Duration) {
	window = time.Duration(s.cfg.AccountSpreadMinutes) * time.Minute
	if window <= 0 {
		return 0, 0
	}
	minGap = time.Duration(s.cfg.AccountSpreadMinGapSeconds) * time.Second
	if minGap < 0 {
		minGap = 0
	}
	if minGap > window {
		minGap = window
	}
	return window, minGap
}

// spreadPlan 一批账号当天的执行偏移。base 是计划生成时刻，偏移相对它计算。
type spreadPlan struct {
	base    time.Time
	offsets map[string]time.Duration
}

// planSpread 计算本批账号的确定性偏移；窗口关闭时返回 nil（调用方直接立即执行）。
//
// 同一天、同一 kind、同一 uid 的结果与进程重启无关；账号集合变化只影响
// 「冲突推开」的先后，不会让不冲突的账号漂移。
func (s *Scheduler) planSpread(kind taskKind, uids []string) *spreadPlan {
	window, minGap := s.spreadSettings()
	if window <= 0 || len(uids) == 0 {
		return nil
	}
	now := time.Now()
	day := now.Format("2006-01-02")
	type pair struct {
		uid string
		off time.Duration
	}
	pairs := make([]pair, 0, len(uids))
	for _, uid := range uids {
		h := spreadHash(uid, kind, day)
		pairs = append(pairs, pair{uid: uid, off: time.Duration(h % uint64(window))})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].off != pairs[j].off {
			return pairs[i].off < pairs[j].off
		}
		return pairs[i].uid < pairs[j].uid
	})
	// 冲突推开：只把落在前一个账号 minGap 内的后者往后推，其余保持随机形态。
	prev := time.Duration(-1)
	for i := range pairs {
		if prev >= 0 && pairs[i].off < prev+minGap {
			pairs[i].off = prev + minGap
		}
		if pairs[i].off > window {
			pairs[i].off = window
		}
		prev = pairs[i].off
	}
	offsets := make(map[string]time.Duration, len(pairs))
	for _, p := range pairs {
		offsets[p.uid] = p.off
	}
	return &spreadPlan{base: now, offsets: offsets}
}

// wait 等到该账号的计划时刻；计划为 nil（窗口关闭/手动入口）时立即返回。
func (p *spreadPlan) wait(uid string) {
	if p == nil {
		return
	}
	off, ok := p.offsets[uid]
	if !ok {
		return
	}
	if d := time.Until(p.base.Add(off)); d > 0 {
		time.Sleep(d)
	}
}

func spreadHash(uid string, kind taskKind, day string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(spreadHashSalt))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(uid))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(string(rune('0' + int(kind)))))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(day))
	return h.Sum64()
}
