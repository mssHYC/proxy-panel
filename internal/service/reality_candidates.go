package service

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptrace"
	"slices"
	"strings"
	"sync"
	"time"

	"proxy-panel/internal/kernel"
)

type RealityCandidate struct {
	Host        string `json:"host"`
	Status      string `json:"status"`
	Message     string `json:"message"`
	LatencyMS   *int64 `json:"latency_ms,omitempty"`
	Address     string `json:"address,omitempty"`
	CDNProvider string `json:"cdn_provider"`
	CDNEvidence string `json:"cdn_evidence,omitempty"`
}

type candidateResolver interface {
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
	LookupCNAME(context.Context, string) (string, error)
}

func CheckRealityCandidates(ctx context.Context, hosts []string) ([]RealityCandidate, error) {
	return checkRealityCandidates(ctx, hosts, net.DefaultResolver, (&net.Dialer{Timeout: 2 * time.Second}).DialContext, nil)
}

func checkRealityCandidates(ctx context.Context, hosts []string, resolver candidateResolver, dial func(context.Context, string, string) (net.Conn, error), roots *tls.Config) ([]RealityCandidate, error) {
	if len(hosts) == 0 || len(hosts) > 8 {
		return nil, fmt.Errorf("每次可检查 1 至 8 个域名")
	}
	var names []string
	for _, host := range hosts {
		host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
		if !slices.Contains(names, host) {
			names = append(names, host)
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	results := make([]RealityCandidate, len(names))
	var wg sync.WaitGroup
	slots := make(chan struct{}, 3)
	for i, host := range names {
		slots <- struct{}{}
		wg.Go(func() {
			defer func() { <-slots }()
			results[i] = probeRealityCandidate(ctx, host, resolver, dial, roots)
		})
	}
	wg.Wait()
	return results, nil
}

func candidateHost(host string) bool {
	// Only fully qualified DNS names are accepted. An absolute lookup below
	// bypasses resolver search suffixes for multi-label names too.
	if len(host) > 253 || !strings.Contains(host, ".") || strings.HasSuffix(host, ".") || net.ParseIP(host) != nil {
		return false
	}
	tld := host[strings.LastIndexByte(host, '.')+1:]
	if strings.Trim(tld, "0123456789") == "" || slices.Contains([]string{"localhost", "local", "localdomain", "internal", "lan", "onion", "arpa"}, tld) {
		return false
	}
	parsed, _, err := kernel.ParseRealityTarget(host)
	return err == nil && parsed == host
}

func probeRealityCandidate(ctx context.Context, host string, resolver candidateResolver, dial func(context.Context, string, string) (net.Conn, error), roots *tls.Config) RealityCandidate {
	result := RealityCandidate{Host: host, CDNProvider: "unknown"}
	fail := func(status, message string) RealityCandidate {
		result.Status, result.Message = status, message
		return result
	}
	if !candidateHost(host) {
		return fail("invalid_host", "请输入完整的公网域名")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	failure := func(status, message string, err error) RealityCandidate {
		var timeout net.Error
		if ctx.Err() != nil || errors.As(err, &timeout) && timeout.Timeout() {
			return fail("timeout", "检查已超时或取消")
		}
		return fail(status, message)
	}
	if ctx.Err() != nil {
		return fail("timeout", "检查已超时或取消")
	}
	ips, err := resolver.LookupIPAddr(ctx, host+".")
	if err != nil || len(ips) == 0 || len(ips) > 64 {
		return failure("dns_error", "域名解析失败或地址数量超出限制", err)
	}
	for _, addr := range ips {
		if addr.Zone != "" || !kernel.PublicRealityIP(addr.IP) {
			return fail("blocked_address", "DNS 含有本机、内网或保留地址")
		}
	}
	// CNAME is observational only. Always dial a numeric address from the
	// vetted A/AAAA answer, even if the CNAME lookup observes a DNS change.
	dnsCtx, stopDNS := context.WithTimeout(ctx, 750*time.Millisecond)
	cname, _ := resolver.LookupCNAME(dnsCtx, host+".")
	stopDNS()
	result.CDNProvider, result.CDNEvidence = candidateCDN(cname, nil)

	// GotConn runs after TCP and TLS complete, before waiting for HTTP headers.
	type peer struct {
		ip string
		ms int64
	}
	connected := make(chan peer, 1)
	start := time.Now()
	ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{GotConn: func(info httptrace.GotConnInfo) {
		ip, _, _ := net.SplitHostPort(info.Conn.RemoteAddr().String())
		select {
		case connected <- peer{ip, time.Since(start).Milliseconds()}:
		default:
		}
	}})
	resp, err := realityHead(ctx, host, "443", ips[:min(len(ips), 4)], dial, roots)
	select {
	case p := <-connected:
		result.Address, result.LatencyMS = p.ip, &p.ms
	default:
	}
	if err != nil {
		var op *net.OpError
		if errors.As(err, &op) && op.Op == "dial" {
			return failure("connect_error", "无法连接到目标的 TCP 443 端口", err)
		}
		if result.LatencyMS != nil {
			return failure("http_error", "TLS 已连接，未收到有效的 HTTP 响应", err)
		}
		return failure("tls_error", "TLS 1.3 握手或证书域名校验失败", err)
	}
	if provider, evidence := candidateCDN(cname, resp.Header); evidence != "" {
		result.CDNProvider, result.CDNEvidence = provider, evidence
	}
	if resp.TLS == nil || resp.TLS.Version != tls.VersionTLS13 || resp.TLS.NegotiatedProtocol != "h2" || resp.ProtoMajor != 2 {
		return fail("no_h2", "目标未协商 HTTP/2")
	}
	if resp.StatusCode >= 500 || resp.StatusCode == http.StatusMisdirectedRequest {
		return fail("http_error", fmt.Sprintf("TLS 和 HTTP/2 检查通过，网站返回 HTTP %d", resp.StatusCode))
	}
	result.Status, result.Message = "compatible", fmt.Sprintf("TLS 1.3、证书和 HTTP/2 检查通过（HTTP %d）", resp.StatusCode)
	return result
}

func candidateCDN(cname string, h http.Header) (string, string) {
	for _, marker := range []struct{ header, provider string }{
		{"CF-Ray", "Cloudflare"}, {"X-Amz-Cf-Id", "CloudFront"}, {"Fastly-Debug-Digest", "Fastly"},
	} {
		if h.Get(marker.header) != "" {
			return marker.provider, "HTTP " + marker.header
		}
	}
	switch strings.ToLower(strings.TrimSpace(h.Get("Server"))) {
	case "cloudflare":
		return "Cloudflare", "HTTP Server: cloudflare"
	case "akamaighost":
		return "Akamai", "HTTP Server: AkamaiGHost"
	}
	cname = strings.TrimSuffix(strings.ToLower(cname), ".")
	for _, marker := range []struct{ suffix, provider string }{
		{"cloudfront.net", "CloudFront"}, {"cdn.cloudflare.net", "Cloudflare"},
		{"edgesuite.net", "Akamai"}, {"edgekey.net", "Akamai"}, {"akamaized.net", "Akamai"}, {"akamaiedge.net", "Akamai"}, {"akamai.net", "Akamai"},
		{"fastly.net", "Fastly"},
	} {
		if cname == marker.suffix || strings.HasSuffix(cname, "."+marker.suffix) {
			return marker.provider, "DNS " + cname
		}
	}
	for _, key := range []string{"Via", "X-Cache", "X-Served-By"} {
		if h.Get(key) != "" {
			return "unknown", "HTTP " + key
		}
	}
	return "unknown", ""
}
