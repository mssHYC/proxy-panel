package firewall

import (
	"context"
	"strings"
	"testing"
)

func TestFirewalldAllow_AddThenReload(t *testing.T) {
	fr := &fakeRun{}
	b := newFirewalldBackend(fr.run)
	if err := b.Allow(context.Background(), 4443); err != nil {
		t.Fatalf("Allow returned error: %v", err)
	}
	if len(fr.calls) != 3 {
		t.Fatalf("expected 3 calls (tcp add, udp add, reload), got %d: %v",
			len(fr.calls), fr.calls)
	}
	expect := []string{
		"firewall-cmd --permanent --add-port=4443/tcp",
		"firewall-cmd --permanent --add-port=4443/udp",
		"firewall-cmd --reload",
	}
	for i, want := range expect {
		if got := strings.Join(fr.calls[i], " "); got != want {
			t.Errorf("call %d: want %q got %q", i, want, got)
		}
	}
}

func TestFirewalldRevoke_RemoveThenReload(t *testing.T) {
	fr := &fakeRun{}
	b := newFirewalldBackend(fr.run)
	if err := b.Revoke(context.Background(), 4443); err != nil {
		t.Fatalf("Revoke returned error: %v", err)
	}
	if len(fr.calls) != 3 {
		t.Fatalf("expected 3 calls, got %d", len(fr.calls))
	}
	expect := []string{
		"firewall-cmd --permanent --remove-port=4443/tcp",
		"firewall-cmd --permanent --remove-port=4443/udp",
		"firewall-cmd --reload",
	}
	for i, want := range expect {
		if got := strings.Join(fr.calls[i], " "); got != want {
			t.Errorf("call %d: want %q got %q", i, want, got)
		}
	}
}

// firewalld 对已存在规则返回 stdout "ALREADY_ENABLED" + 非零退出；视为成功
func TestFirewalldAllow_IgnoresAlreadyEnabled(t *testing.T) {
	fr := &fakeRun{
		stdouts: [][]byte{
			[]byte("Warning: ALREADY_ENABLED: 4443:tcp\nsuccess\n"),
			[]byte("Warning: ALREADY_ENABLED: 4443:udp\nsuccess\n"),
			[]byte("success\n"),
		},
		errs: []error{errFake("exit 12"), errFake("exit 12"), nil},
	}
	b := newFirewalldBackend(fr.run)
	if err := b.Allow(context.Background(), 4443); err != nil {
		t.Fatalf("should tolerate ALREADY_ENABLED, got: %v", err)
	}
}

// 不存在规则的 remove 返回 NOT_ENABLED + 非零；视为成功
func TestFirewalldRevoke_IgnoresNotEnabled(t *testing.T) {
	fr := &fakeRun{
		stdouts: [][]byte{
			[]byte("Warning: NOT_ENABLED: 4443:tcp\n"),
			[]byte("Warning: NOT_ENABLED: 4443:udp\n"),
			[]byte("success\n"),
		},
		errs: []error{errFake("exit 12"), errFake("exit 12"), nil},
	}
	b := newFirewalldBackend(fr.run)
	if err := b.Revoke(context.Background(), 4443); err != nil {
		t.Fatalf("should tolerate NOT_ENABLED, got: %v", err)
	}
}

// 真实错误（如未启动）必须返回
func TestFirewalldAllow_PropagatesRealError(t *testing.T) {
	fr := &fakeRun{
		stdouts: [][]byte{[]byte("FirewallD is not running\n")},
		errs:    []error{errFake("exit status 252")},
	}
	b := newFirewalldBackend(fr.run)
	if err := b.Allow(context.Background(), 4443); err == nil {
		t.Fatalf("expected error, got nil")
	}
}
