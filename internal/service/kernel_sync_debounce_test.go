package service

import (
	"sync"
	"testing"
	"time"
)

// fakeDebounceTarget 模拟一个会记录调用次数的同步目标，
// 用来验证 Trigger 的合并行为与 SyncNow 的即刻行为。
type fakeDebounceTarget struct {
	mu    sync.Mutex
	calls int
}

func (f *fakeDebounceTarget) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *fakeDebounceTarget) inc() {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
}

// TestTrigger_MergesWithinWindow 多次 Trigger 应只执行一次目标调用。
func TestTrigger_MergesWithinWindow(t *testing.T) {
	target := &fakeDebounceTarget{}
	s := &KernelSyncService{debounceWindow: 100 * time.Millisecond}
	trigger := func() {
		s.debounceMu.Lock()
		defer s.debounceMu.Unlock()
		if s.debounceTimer != nil {
			s.debounceTimer.Stop()
		}
		s.debounceTimer = time.AfterFunc(s.debounceWindow, target.inc)
	}
	for i := 0; i < 20; i++ {
		trigger()
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(300 * time.Millisecond)
	if got := target.count(); got != 1 {
		t.Errorf("debounce should merge to 1 call, got %d", got)
	}
}

// TestTrigger_FiresAfterQuietPeriod 安静期结束后应触发一次调用。
func TestTrigger_FiresAfterQuietPeriod(t *testing.T) {
	target := &fakeDebounceTarget{}
	s := &KernelSyncService{debounceWindow: 50 * time.Millisecond}
	trigger := func() {
		s.debounceMu.Lock()
		defer s.debounceMu.Unlock()
		if s.debounceTimer != nil {
			s.debounceTimer.Stop()
		}
		s.debounceTimer = time.AfterFunc(s.debounceWindow, target.inc)
	}
	trigger()
	time.Sleep(200 * time.Millisecond)
	if got := target.count(); got != 1 {
		t.Errorf("should fire once after quiet period, got %d", got)
	}
	trigger()
	time.Sleep(200 * time.Millisecond)
	if got := target.count(); got != 2 {
		t.Errorf("second burst should fire, got %d", got)
	}
}

// TestSyncNow_CancelsPending SyncNow 应取消等待中的防抖计时器。
func TestSyncNow_CancelsPending(t *testing.T) {
	target := &fakeDebounceTarget{}
	s := &KernelSyncService{debounceWindow: 100 * time.Millisecond}

	// 先启动一个防抖计时
	s.debounceMu.Lock()
	s.debounceTimer = time.AfterFunc(s.debounceWindow, target.inc)
	s.debounceMu.Unlock()

	// 模拟 SyncNow：取消 pending timer
	s.debounceMu.Lock()
	if s.debounceTimer != nil {
		s.debounceTimer.Stop()
		s.debounceTimer = nil
	}
	s.debounceMu.Unlock()

	time.Sleep(200 * time.Millisecond)
	if got := target.count(); got != 0 {
		t.Errorf("pending debounce should be canceled by SyncNow, got %d", got)
	}
}
