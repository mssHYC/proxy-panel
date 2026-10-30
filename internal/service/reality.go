package service

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"reflect"
	"strings"
	"time"

	"proxy-panel/internal/kernel"
)

type RealityCheck struct {
	Target      string   `json:"target"`
	ServerNames []string `json:"server_names"`
	Warnings    []string `json:"warnings"`
}

// CheckRealityTarget checks from the VPS, not the browser's network. Pin the
// validated DNS answers when dialing so DNS rebinding cannot reach management.
func CheckRealityTarget(ctx context.Context, settings map[string]interface{}) (*RealityCheck, error) {
	return checkRealityTarget(ctx, settings, net.DefaultResolver.LookupIPAddr, (&net.Dialer{Timeout: 3 * time.Second}).DialContext, nil)
}

func checkRealityTarget(ctx context.Context, settings map[string]interface{}, lookup func(context.Context, string) ([]net.IPAddr, error), dial func(context.Context, string, string) (net.Conn, error), roots *tls.Config) (*RealityCheck, error) {
	target, names, err := kernel.RealityTarget(settings)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	host, port, _ := net.SplitHostPort(target)
	ips, err := lookup(ctx, host)
	if err != nil || len(ips) == 0 {
		return nil, fmt.Errorf("Reality 目标 DNS 解析失败")
	}
	for _, ip := range ips {
		if !kernel.PublicRealityIP(ip.IP) {
			return nil, fmt.Errorf("Reality 目标解析到本机或非公网地址，已拒绝检查")
		}
	}
	result := &RealityCheck{Target: target, ServerNames: names, Warnings: []string{}}
	for _, name := range names {
		tc := &tls.Config{MinVersion: tls.VersionTLS13, ServerName: name}
		if roots != nil {
			tc.RootCAs = roots.RootCAs
		}
		transport := &http.Transport{
			Proxy: nil, ForceAttemptHTTP2: true, TLSClientConfig: tc,
			TLSHandshakeTimeout: 3 * time.Second, MaxResponseHeaderBytes: 32 * 1024,
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				var last error
				for _, ip := range ips {
					conn, e := dial(ctx, "tcp", net.JoinHostPort(ip.IP.String(), port))
					if e == nil {
						return conn, nil
					}
					last = e
				}
				return nil, last
			},
		}
		client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		req, _ := http.NewRequestWithContext(ctx, http.MethodHead, "https://"+net.JoinHostPort(name, port)+"/", nil)
		resp, err := client.Do(req)
		if err != nil {
			transport.CloseIdleConnections()
			return nil, fmt.Errorf("Reality 目标 %s 检查失败（连通性、TLS 1.3 或证书/SNI）: %w", name, err)
		}
		resp.Body.Close()
		transport.CloseIdleConnections()
		if resp.TLS == nil || resp.TLS.Version != tls.VersionTLS13 || resp.TLS.NegotiatedProtocol != "h2" {
			return nil, fmt.Errorf("Reality 目标 %s 必须支持 TLS 1.3 和 HTTP/2", name)
		}
		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			result.Warnings = append(result.Warnings, name+" 首页发生跳转，建议选择最终网站域名作为目标")
		}
		server := strings.ToLower(resp.Header.Get("Server"))
		if resp.Header.Get("CF-Ray") != "" || resp.Header.Get("Via") != "" || resp.Header.Get("X-Cache") != "" || resp.Header.Get("X-Served-By") != "" || strings.Contains(server, "cloudflare") || strings.Contains(server, "akamai") {
			result.Warnings = append(result.Warnings, name+" 检测到 CDN/共享边缘特征；Reality 回落可能被借用访问同 IP 的其他站点，请评估目标与流量限制")
		}
	}
	result.Warnings = append(result.Warnings, "检查仅验证当前连通性和协议能力，不能保证目标长期可用或识别所有 CDN")
	return result, nil
}

func (s *NodeService) validateRealityChange(oldSettings, newSettings string, enable bool) error {
	var old, next map[string]interface{}
	_ = json.Unmarshal([]byte(oldSettings), &old)
	_ = json.Unmarshal([]byte(newSettings), &next)
	if next["security"] != "reality" {
		return nil
	}
	target, names, err := kernel.RealityTarget(next)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidNodeSettings, err)
	}
	oldTarget, oldNames, _ := kernel.RealityTarget(old)
	if !enable && old["security"] == "reality" && target == oldTarget && reflect.DeepEqual(names, oldNames) {
		return nil
	}
	check := s.realityCheck
	if check == nil {
		check = CheckRealityTarget
	}
	if _, err := check(context.Background(), next); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidNodeSettings, err)
	}
	return nil
}
