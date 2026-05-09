package service

import (
	"bytes"
	"context"
	"net"
	"testing"
	"time"

	"github.com/quic-go/quic-go"
)

func TestSalamanderRoundtrip(t *testing.T) {
	o := newSalamander("hunter2")
	in := []byte("the quick brown fox jumps over 13 lazy QUIC packets")
	wire := make([]byte, len(in)+salamanderSaltLen+8)
	n, err := o.obfuscate(in, wire)
	if err != nil {
		t.Fatalf("obfuscate: %v", err)
	}
	if bytes.Contains(wire[:n], in) {
		t.Fatalf("plaintext leaked into obfuscated wire")
	}
	out := make([]byte, n)
	m, err := o.deobfuscate(wire[:n], out)
	if err != nil {
		t.Fatalf("deobfuscate: %v", err)
	}
	if !bytes.Equal(out[:m], in) {
		t.Fatalf("roundtrip mismatch: got %q want %q", out[:m], in)
	}
}

func TestSalamanderWrongPasswordFails(t *testing.T) {
	a := newSalamander("right")
	b := newSalamander("wrong")
	in := []byte("hello world")
	wire := make([]byte, len(in)+salamanderSaltLen)
	n, _ := a.obfuscate(in, wire)
	out := make([]byte, n)
	m, err := b.deobfuscate(wire[:n], out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bytes.Equal(out[:m], in) {
		t.Fatalf("wrong-password decrypt should not recover plaintext")
	}
}

// 起一个被 salamander 包裹的 QUIC 服务端，验证带 obfs 的 probeQUIC 能正确握手。
func TestProbeQUIC_OnlineWithSalamander(t *testing.T) {
	const password = "shared-secret"

	srvUDP, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}
	srvWrapped := newObfsPacketConn(srvUDP, newSalamander(password))
	tr := &quic.Transport{Conn: srvWrapped}
	defer tr.Close()
	tlsConf := generateSelfSignedTLS(t, []string{"h3"})
	ln, err := tr.Listen(tlsConf, &quic.Config{})
	if err != nil {
		t.Fatalf("listen quic: %v", err)
	}
	defer ln.Close()

	go func() {
		for {
			c, err := ln.Accept(context.Background())
			if err != nil {
				return
			}
			c.CloseWithError(0, "")
		}
	}()

	port := srvUDP.LocalAddr().(*net.UDPAddr).Port
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := probeQUIC(ctx, "127.0.0.1", port, quicProbeOpts{
		obfsKind:     "salamander",
		obfsPassword: password,
	}); err != nil {
		t.Fatalf("expected online via salamander, got err: %v", err)
	}
}

// 没有 obfs 的 probe 打到一个开了 salamander 的服务器上，应判离线（超时）。
// 这正是用户当前线上的故障场景。
func TestProbeQUIC_BareProbeAgainstSalamanderServerOffline(t *testing.T) {
	const password = "shared-secret"

	srvUDP, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}
	srvWrapped := newObfsPacketConn(srvUDP, newSalamander(password))
	tr := &quic.Transport{Conn: srvWrapped}
	defer tr.Close()
	tlsConf := generateSelfSignedTLS(t, []string{"h3"})
	ln, err := tr.Listen(tlsConf, &quic.Config{})
	if err != nil {
		t.Fatalf("listen quic: %v", err)
	}
	defer ln.Close()

	port := srvUDP.LocalAddr().(*net.UDPAddr).Port
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	if err := probeQUIC(ctx, "127.0.0.1", port, quicProbeOpts{}); err == nil {
		t.Fatalf("bare probe against obfs server must fail (this is the bug we are fixing)")
	}
}

func TestParseObfsFromSettings(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		wantKind string
		wantPwd  string
	}{
		{"empty", "", "", ""},
		{"no obfs", `{"foo":"bar"}`, "", ""},
		{"salamander", `{"obfs":"salamander","obfs_password":"abc"}`, "salamander", "abc"},
		{"malformed", `not-json`, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			k, p := parseObfsFromSettings(tc.in)
			if k != tc.wantKind || p != tc.wantPwd {
				t.Fatalf("got (%q,%q), want (%q,%q)", k, p, tc.wantKind, tc.wantPwd)
			}
		})
	}
}
