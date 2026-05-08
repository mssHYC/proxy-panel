package service

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"strconv"

	"github.com/quic-go/quic-go"
)

// probeQUIC 探测目标是否在监听 QUIC（Hysteria2 / TUIC）。
//
// 思路：发起一次最小握手，所有"服务器有回包"的结果都视为在线，包括 ALPN 不匹配、
// 证书拒绝等 CRYPTO 错误；只有 idle / handshake timeout / ctx 超时才视为离线。
// 这样可以在不知道 server cert / ALPN 的前提下判断 QUIC 端口可达性。
//
// 调用方负责传入带 timeout 的 ctx。
func probeQUIC(ctx context.Context, host string, port int) error {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	tlsConf := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"h3", "tuic"},
	}
	conn, err := quic.DialAddr(ctx, addr, tlsConf, &quic.Config{})
	if err == nil {
		conn.CloseWithError(0, "")
		return nil
	}
	if isQUICServerReplyErr(err) {
		return nil
	}
	return err
}

// isQUICServerReplyErr 判断 dial 失败是否由"服务器有回包"引起。
// 仅 IdleTimeout / HandshakeTimeout / context 超时视为无回包（离线）。
func isQUICServerReplyErr(err error) bool {
	if err == nil {
		return true
	}
	var idle *quic.IdleTimeoutError
	if errors.As(err, &idle) {
		return false
	}
	var hs *quic.HandshakeTimeoutError
	if errors.As(err, &hs) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return false
	}
	return true
}
