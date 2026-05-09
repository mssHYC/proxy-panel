package service

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"strconv"

	"github.com/quic-go/quic-go"
)

// quicProbeOpts 让调用方传入与节点 settings 对应的混淆参数。obfsKind 为空时
// 走裸 QUIC；obfsKind="salamander" 时按 Hysteria2 salamander 规则封装 UDP 包，
// 否则当作未识别混淆，直接判离线（盲发裸包必然超时，没必要再等）。
type quicProbeOpts struct {
	obfsKind     string
	obfsPassword string
}

// probeQUIC 探测目标是否在监听 QUIC（Hysteria2 / TUIC）。
//
// 仅当能正向证明"服务器有 QUIC 回包"（握手成功 / 远端发回 CRYPTO_ERROR / 应用层
// CONNECTION_CLOSE / Version Negotiation / Stateless Reset）时返回 nil；
// 其余错误（idle/handshake timeout、ctx 超时、DNS 失败、系统级网络错误如
// ECONNREFUSED/ENETUNREACH/EHOSTUNREACH）一律视为离线。
//
// 注意：QUIC 探测只确认目标 UDP 端口上有 QUIC 服务在响应，不验证 Hy2/TUIC
// 的账号/密码/业务可用性。
//
// 调用方负责传入带 timeout 的 ctx。
func probeQUIC(ctx context.Context, host string, port int, opts quicProbeOpts) error {
	tlsConf := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"h3", "tuic"},
	}

	if opts.obfsKind == "" {
		addr := net.JoinHostPort(host, strconv.Itoa(port))
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

	if opts.obfsKind != "salamander" {
		return fmt.Errorf("unsupported obfs %q", opts.obfsKind)
	}

	udpAddr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return err
	}
	pc, err := net.ListenUDP("udp", nil)
	if err != nil {
		return err
	}
	defer pc.Close()
	wrapped := newObfsPacketConn(pc, newSalamander(opts.obfsPassword))

	conn, err := quic.Dial(ctx, wrapped, udpAddr, tlsConf, &quic.Config{})
	if err == nil {
		conn.CloseWithError(0, "")
		return nil
	}
	if isQUICServerReplyErr(err) {
		return nil
	}
	return err
}

// isQUICServerReplyErr 仅当错误本身能证明"服务器有回包"时才返回 true。
// 用正向白名单（quic.TransportError / ApplicationError / VersionNegotiationError /
// StatelessResetError）判断，避免把本地 DNS、network unreachable、UDP port
// unreachable 等错误误判为在线。
//
// TransportError 既覆盖远端 CRYPTO_ERROR（ALPN mismatch、服务器主动 close），
// 也覆盖本地 TLS 验证产生的 CRYPTO_ERROR；后者意味着我们已经从服务器收到
// 证书消息，因此同样属于"有回包"。
func isQUICServerReplyErr(err error) bool {
	if err == nil {
		return true
	}
	var tErr *quic.TransportError
	if errors.As(err, &tErr) {
		return true
	}
	var aErr *quic.ApplicationError
	if errors.As(err, &aErr) {
		return true
	}
	var vErr *quic.VersionNegotiationError
	if errors.As(err, &vErr) {
		return true
	}
	var sErr *quic.StatelessResetError
	if errors.As(err, &sErr) {
		return true
	}
	return false
}
