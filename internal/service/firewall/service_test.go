package firewall

import (
	"context"
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

func (f *fakeBackend) Name() string                              { return f.name }
func (f *fakeBackend) Available(ctx context.Context) error       { return nil }
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
