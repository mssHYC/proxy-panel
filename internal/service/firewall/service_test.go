package firewall

import (
	"context"
	"strings"
	"testing"
)

// fakeBackend 记录调用轨迹，便于 Service 行为断言
type fakeBackend struct {
	name      string
	allows    []int
	revokes   []int
	allowErr  error
	revokeErr error
}

func (f *fakeBackend) Name() string                        { return f.name }
func (f *fakeBackend) Available(ctx context.Context) error { return nil }
func (f *fakeBackend) Allow(ctx context.Context, port int) error {
	f.allows = append(f.allows, port)
	return f.allowErr
}
func (f *fakeBackend) Revoke(ctx context.Context, port int) error {
	f.revokes = append(f.revokes, port)
	return f.revokeErr
}

// fakeNotifier 记录 SendAll 调用
type fakeNotifier struct {
	messages []string
}

func (n *fakeNotifier) SendAll(msg string) { n.messages = append(n.messages, msg) }

func TestService_Disabled_AllMethodsNoop(t *testing.T) {
	b := &fakeBackend{name: "fake"}
	n := &fakeNotifier{}
	s := &Service{backend: b, enabled: false, notify: n}

	if s.Enabled() {
		t.Fatalf("expected Enabled=false")
	}
	if err := s.Allow(1234); err != nil {
		t.Fatalf("Allow returned error: %v", err)
	}
	if err := s.Revoke(1234); err != nil {
		t.Fatalf("Revoke returned error: %v", err)
	}
	s.EnsureAll(context.Background(), []int{1, 2, 3})

	if len(b.allows)+len(b.revokes) > 0 {
		t.Fatalf("backend called while disabled: allows=%v revokes=%v", b.allows, b.revokes)
	}
	if len(n.messages) > 0 {
		t.Fatalf("notifier called while disabled: %v", n.messages)
	}
}

func TestService_Allow_Success(t *testing.T) {
	b := &fakeBackend{name: "fake"}
	n := &fakeNotifier{}
	s := &Service{backend: b, enabled: true, notify: n}

	if err := s.Allow(4443); err != nil {
		t.Fatalf("Allow: %v", err)
	}
	if len(b.allows) != 1 || b.allows[0] != 4443 {
		t.Fatalf("want allow 4443, got %v", b.allows)
	}
	if len(n.messages) != 0 {
		t.Fatalf("notify should not be called on success: %v", n.messages)
	}
}

func TestService_Allow_BackendFailure_TriggersNotify(t *testing.T) {
	b := &fakeBackend{name: "fake", allowErr: errFake("ufw boom")}
	n := &fakeNotifier{}
	s := &Service{backend: b, enabled: true, notify: n}

	err := s.Allow(4443)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if len(n.messages) != 1 {
		t.Fatalf("expected 1 notify, got %d: %v", len(n.messages), n.messages)
	}
	if !strings.Contains(n.messages[0], "放行") || !strings.Contains(n.messages[0], "4443") {
		t.Errorf("notify text lacks expected context: %q", n.messages[0])
	}
}

func TestService_Revoke_BackendFailure_TriggersNotify(t *testing.T) {
	b := &fakeBackend{name: "fake", revokeErr: errFake("firewalld down")}
	n := &fakeNotifier{}
	s := &Service{backend: b, enabled: true, notify: n}

	if err := s.Revoke(4443); err == nil {
		t.Fatalf("expected error, got nil")
	}
	if len(n.messages) != 1 || !strings.Contains(n.messages[0], "关闭") {
		t.Errorf("notify text unexpected: %v", n.messages)
	}
}

func TestService_EnsureAll_ContinuesOnFailure(t *testing.T) {
	b := &fakeBackend{
		name:     "fake",
		allowErr: errFake("backend down"),
	}
	n := &fakeNotifier{}
	s := &Service{backend: b, enabled: true, notify: n}

	s.EnsureAll(context.Background(), []int{10, 20, 30})

	if len(b.allows) != 3 {
		t.Fatalf("expected 3 allows even with failures, got %v", b.allows)
	}
}
