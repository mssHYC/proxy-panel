package firewall

import (
	"context"
	"strings"
	"testing"
)

// fakeRun 记录每次调用的完整命令，并按预设返回结果
type fakeRun struct {
	calls    [][]string
	stdouts  [][]byte
	stderrs  [][]byte
	errs     []error
	callIdx  int
}

func (f *fakeRun) run(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	f.calls = append(f.calls, append([]string{name}, args...))
	i := f.callIdx
	f.callIdx++
	var out, errb []byte
	var err error
	if i < len(f.stdouts) {
		out = f.stdouts[i]
	}
	if i < len(f.stderrs) {
		errb = f.stderrs[i]
	}
	if i < len(f.errs) {
		err = f.errs[i]
	}
	return out, errb, err
}

func TestUFWAllow_EmitsTCPAndUDP(t *testing.T) {
	fr := &fakeRun{}
	b := newUFWBackend(fr.run)
	if err := b.Allow(context.Background(), 4443); err != nil {
		t.Fatalf("Allow returned error: %v", err)
	}
	if len(fr.calls) != 2 {
		t.Fatalf("expected 2 calls, got %d: %v", len(fr.calls), fr.calls)
	}
	if got := strings.Join(fr.calls[0], " "); got != "ufw allow 4443/tcp" {
		t.Errorf("call 0: want 'ufw allow 4443/tcp', got %q", got)
	}
	if got := strings.Join(fr.calls[1], " "); got != "ufw allow 4443/udp" {
		t.Errorf("call 1: want 'ufw allow 4443/udp', got %q", got)
	}
}

func TestUFWRevoke_EmitsTCPAndUDP(t *testing.T) {
	fr := &fakeRun{}
	b := newUFWBackend(fr.run)
	if err := b.Revoke(context.Background(), 4443); err != nil {
		t.Fatalf("Revoke returned error: %v", err)
	}
	if len(fr.calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(fr.calls))
	}
	if got := strings.Join(fr.calls[0], " "); got != "ufw delete allow 4443/tcp" {
		t.Errorf("call 0: want 'ufw delete allow 4443/tcp', got %q", got)
	}
	if got := strings.Join(fr.calls[1], " "); got != "ufw delete allow 4443/udp" {
		t.Errorf("call 1: want 'ufw delete allow 4443/udp', got %q", got)
	}
}

// ufw 对 delete 不存在的规则返回非零 + stderr "Could not delete non-existent rule"
// 该场景视为成功
func TestUFWRevoke_IgnoresNonExistentRule(t *testing.T) {
	fr := &fakeRun{
		stderrs: [][]byte{
			[]byte("Could not delete non-existent rule\n"),
			[]byte("Could not delete non-existent rule (v6)\n"),
		},
		errs: []error{errFake("exit status 1"), errFake("exit status 1")},
	}
	b := newUFWBackend(fr.run)
	if err := b.Revoke(context.Background(), 4443); err != nil {
		t.Fatalf("Revoke should tolerate non-existent rule, got: %v", err)
	}
}

// 其他错误必须原样透出
func TestUFWAllow_PropagatesRealError(t *testing.T) {
	fr := &fakeRun{
		stderrs: [][]byte{[]byte("ERROR: cannot bind to management socket\n")},
		errs:    []error{errFake("exit status 2")},
	}
	b := newUFWBackend(fr.run)
	if err := b.Allow(context.Background(), 4443); err == nil {
		t.Fatalf("expected error, got nil")
	}
}

// errFake 避免引 errors 包只为一次 New
type errFake string

func (e errFake) Error() string { return string(e) }
