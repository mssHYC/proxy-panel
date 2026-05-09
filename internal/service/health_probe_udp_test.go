package service

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net"
	"testing"
	"time"

	"github.com/quic-go/quic-go"
)

func generateSelfSignedTLS(t *testing.T, alpn []string) *tls.Config {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "probe-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}},
		NextProtos:   alpn,
	}
}

func TestProbeQUIC_Online(t *testing.T) {
	tlsConf := generateSelfSignedTLS(t, []string{"h3"})
	ln, err := quic.ListenAddr("127.0.0.1:0", tlsConf, &quic.Config{})
	if err != nil {
		t.Fatalf("listen: %v", err)
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
	addr := ln.Addr().(*net.UDPAddr)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := probeQUIC(ctx, "127.0.0.1", addr.Port, quicProbeOpts{}); err != nil {
		t.Fatalf("expected online, got err: %v", err)
	}
}

func TestProbeQUIC_OnlineALPNMismatch(t *testing.T) {
	// 服务端只声明一个不在客户端 NextProtos 中的 ALPN，握手会失败但服务器有回包，
	// 应当被识别为在线。
	tlsConf := generateSelfSignedTLS(t, []string{"unknown-proto-x"})
	ln, err := quic.ListenAddr("127.0.0.1:0", tlsConf, &quic.Config{})
	if err != nil {
		t.Fatalf("listen: %v", err)
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
	addr := ln.Addr().(*net.UDPAddr)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := probeQUIC(ctx, "127.0.0.1", addr.Port, quicProbeOpts{}); err != nil {
		t.Fatalf("ALPN 不匹配也应识别为在线，但返回错误: %v", err)
	}
}

func TestProbeQUIC_OfflineDNSFail(t *testing.T) {
	// DNS 解析失败应当判离线，而不是因为"非 timeout"被误判在线。
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := probeQUIC(ctx, "no-such-host.invalid.example.test.", 443, quicProbeOpts{})
	if err == nil {
		t.Fatalf("DNS 失败应返回离线错误")
	}
}

func TestIsQUICServerReplyErr_NetworkErrorsAreOffline(t *testing.T) {
	// *net.OpError / *net.DNSError / 系统级网络错误 / ctx 超时一律判离线。
	cases := []struct {
		name string
		err  error
	}{
		{"net.OpError dial", &net.OpError{Op: "dial", Net: "udp", Err: errors.New("connection refused")}},
		{"net.DNSError", &net.DNSError{Err: "no such host", Name: "x.invalid"}},
		{"plain string err", errors.New("network is unreachable")},
		{"context deadline", context.DeadlineExceeded},
		{"context canceled", context.Canceled},
		{"quic IdleTimeout", &quic.IdleTimeoutError{}},
		{"quic HandshakeTimeout", &quic.HandshakeTimeoutError{}},
	}
	for _, tc := range cases {
		if isQUICServerReplyErr(tc.err) {
			t.Errorf("%s 应判离线，但被识别为在线: %v", tc.name, tc.err)
		}
	}
}

func TestIsQUICServerReplyErr_QUICReplyErrorsAreOnline(t *testing.T) {
	// 真正来自服务器的 QUIC 错误必须识别为在线。
	cases := []struct {
		name string
		err  error
	}{
		{"TransportError remote", &quic.TransportError{Remote: true, ErrorCode: 0x178}}, // CRYPTO_ERROR + alert
		{"TransportError local crypto", &quic.TransportError{Remote: false, ErrorCode: 0x100}},
		{"ApplicationError", &quic.ApplicationError{Remote: true, ErrorCode: 1}},
		{"VersionNegotiationError", &quic.VersionNegotiationError{}},
		{"StatelessResetError", &quic.StatelessResetError{}},
	}
	for _, tc := range cases {
		if !isQUICServerReplyErr(tc.err) {
			t.Errorf("%s 应判在线，但被识别为离线: %v", tc.name, tc.err)
		}
	}
}

func TestProbeQUIC_Offline(t *testing.T) {
	// 抓一个端口然后立刻释放，用于模拟未监听 UDP。
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}
	port := pc.LocalAddr().(*net.UDPAddr).Port
	pc.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	if err := probeQUIC(ctx, "127.0.0.1", port, quicProbeOpts{}); err == nil {
		t.Fatalf("expected offline error, got nil")
	}
}
