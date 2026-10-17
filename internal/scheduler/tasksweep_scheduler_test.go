package scheduler

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// StartTaskSweep 启动后应立刻执行首轮（升级/重启即补领，不必等满一个间隔），
// 之后才进入 ticker 周期。清扫本身幂等，启动首轮与周期轮共用同一 hook。
func TestStartTaskSweepRunsFirstRoundImmediately(t *testing.T) {
	s := New(Config{})
	ran := make(chan struct{}, 1)
	s.SetTaskSweepHook(func() {
		select {
		case ran <- struct{}{}:
		default:
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.StartTaskSweep(ctx, time.Hour)

	select {
	case <-ran:
	case <-time.After(2 * time.Second):
		t.Fatal("StartTaskSweep 启动后未立即执行首轮")
	}
	// 间隔是 1h，测试窗口内不应再触发。
	select {
	case <-ran:
		t.Fatal("1h 间隔内不应重复执行")
	case <-time.After(100 * time.Millisecond):
	}
}

// interval<=0 表示关闭：不启动 goroutine，也不执行首轮。
func TestStartTaskSweepDisabledDoesNotRun(t *testing.T) {
	s := New(Config{})
	var ran int32
	s.SetTaskSweepHook(func() { atomic.AddInt32(&ran, 1) })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.StartTaskSweep(ctx, 0)
	time.Sleep(100 * time.Millisecond)
	if got := atomic.LoadInt32(&ran); got != 0 {
		t.Fatalf("interval<=0 不应启动清扫，hook 调用 %d 次", got)
	}
}
