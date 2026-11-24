package service

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"proxy-panel/internal/kernel"
)

type candidateTestDNS struct {
	lookup func(context.Context, string) ([]net.IPAddr, error)
	cname  string
}

func (d candidateTestDNS) LookupIPAddr(ctx context.Context, name string) ([]net.IPAddr, error) {
	return d.lookup(ctx, name)
}

func (d candidateTestDNS) LookupCNAME(context.Context, string) (string, error) {
	return d.cname, nil
}

func TestRealityCandidatePinnedHTTPS(t *testing.T) {
	for _, tc := range []struct {
		name, host, cname, header, provider, status string
		h2, trust                                   bool
		version                                     uint16
		httpStatus                                  int
	}{
		{"TLS and CDN header", "example.com", "", "CF-Ray", "Cloudflare", "compatible", true, true, tls.VersionTLS13, http.StatusMovedPermanently},
		{"CDN alias", "example.com", "d123.cloudfront.net.", "", "CloudFront", "compatible", true, true, tls.VersionTLS13, http.StatusMovedPermanently},
		{"no provider evidence", "example.com", "notcloudfront.net.", "", "unknown", "compatible", true, true, tls.VersionTLS13, http.StatusMovedPermanently},
		{"wrong name", "wrong.example", "", "", "unknown", "tls_error", true, true, tls.VersionTLS13, http.StatusMovedPermanently},
		{"untrusted certificate", "example.com", "", "", "unknown", "tls_error", true, false, tls.VersionTLS13, http.StatusMovedPermanently},
		{"TLS 1.2 only", "example.com", "", "", "unknown", "tls_error", true, true, tls.VersionTLS12, http.StatusMovedPermanently},
		{"HTTP 1.1 only", "example.com", "", "", "unknown", "no_h2", false, true, tls.VersionTLS13, http.StatusMovedPermanently},
		{"unavailable website", "example.com", "", "", "unknown", "http_error", true, true, tls.VersionTLS13, http.StatusServiceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int32
			srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.Method != "HEAD" || r.URL.Path != "/" || r.TLS.ServerName != tc.host {
					t.Errorf("unexpected request %v %s / SNI %s", r.Method, r.URL.Path, r.TLS.ServerName)
				}
				if tc.header != "" {
					w.Header().Set(tc.header, "fixture")
				}
				w.Header().Set("Location", "https://127.0.0.1/private")
				w.WriteHeader(tc.httpStatus)
			}))
			srv.EnableHTTP2 = tc.h2
			srv.TLS = &tls.Config{MaxVersion: tc.version}
			srv.StartTLS()
			defer srv.Close()
			roots := x509.NewCertPool()
			if tc.trust {
				roots.AddCert(srv.Certificate())
			}
			var lookups, dials atomic.Int32
			resolver := candidateTestDNS{cname: tc.cname, lookup: func(_ context.Context, host string) ([]net.IPAddr, error) {
				if host != tc.host+"." {
					t.Errorf("DNS query for %q", host)
				}
				if lookups.Add(1) > 1 {
					return []net.IPAddr{{IP: net.IPv4(127, 0, 0, 1)}}, nil
				}
				return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
			}}
			dial := func(ctx context.Context, network, addr string) (net.Conn, error) {
				dials.Add(1)
				if addr != "93.184.216.34:443" {
					t.Errorf("unvalidated destination %q", addr)
				}
				return (&net.Dialer{}).DialContext(ctx, network, srv.Listener.Addr().String())
			}
			r, err := checkRealityCandidates(context.Background(), []string{" " + tc.host + ". ", tc.host}, resolver, dial, &tls.Config{RootCAs: roots})
			if err != nil || len(r) != 1 || r[0].Status != tc.status || r[0].CDNProvider != tc.provider || lookups.Load() != 1 || dials.Load() != 1 {
				t.Fatalf("result %+v, err %v, lookups %d, dials %d", r, err, lookups.Load(), dials.Load())
			}
			if tc.status == "compatible" && (r[0].LatencyMS == nil || r[0].Address == "" || requests.Load() != 1) {
				t.Fatalf("latency or single HEAD missing: %+v %d", r[0], requests.Load())
			}
		})
	}
}

func TestRealityCandidatesRejectUnsafeAddresses(t *testing.T) {
	for _, ip := range []string{"::ffff:127.0.0.1", "100.64.0.0", "169.254.169.254", "168.63.129.16", "64:ff9b::7f00:1", "2002:7f00:1::", "2001::1234", "3fff::1", "fd00::1", "ff02::1", "198.18.0.1"} {
		if kernel.PublicRealityIP(net.ParseIP(ip)) {
			t.Errorf("unsafe destination accepted: %s", ip)
		}
	}
	resolver := candidateTestDNS{lookup: func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}, {IP: net.ParseIP("10.10.10.10")}}, nil
	}}
	results, err := checkRealityCandidates(context.Background(), []string{"internal.example"}, resolver, nil, nil)
	if err != nil || results[0].Status != "blocked_address" {
		t.Fatalf("mixed address set: %+v, %v", results, err)
	}
	for _, host := range []string{"example.com:443", "example.com:8443", "https://example.com", "localhost", "foo.localhost", "foo.local", "127.0.0.1", "93.184.216.34", "2001:4860:4860::8888", "example.com..", "*.foo", "user@example.com", "example..org"} {
		if candidateHost(host) {
			t.Errorf("unsafe hostname accepted: %q", host)
		}
	}
}

func TestRealityCandidateBatchBounds(t *testing.T) {
	for _, hosts := range [][]string{nil, make([]string, 9)} {
		if _, err := checkRealityCandidates(context.Background(), hosts, nil, nil, nil); err == nil {
			t.Fatalf("accepted batch of size %d", len(hosts))
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan string, 8)
	resolver := candidateTestDNS{lookup: func(ctx context.Context, host string) ([]net.IPAddr, error) {
		started <- host
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	input := []string{"first.example", "SECOND.EXAMPLE.", "third.example", "First.Example", "fourth.example"}
	done := make(chan []RealityCandidate, 1)
	go func() {
		results, _ := checkRealityCandidates(ctx, input, resolver, nil, nil)
		done <- results
	}()
	for range 3 {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("expected three concurrent checks")
		}
	}
	select {
	case <-started:
		t.Fatal("too many checks started")
	case <-time.After(10 * time.Millisecond):
	}
	cancel()
	select {
	case results := <-done:
		var names []string
		for _, r := range results {
			names = append(names, r.Host)
			if r.Status != "timeout" {
				t.Fatalf("cancelled check: %+v", r)
			}
		}
		if !reflect.DeepEqual(names, []string{"first.example", "second.example", "third.example", "fourth.example"}) {
			t.Fatalf("unexpected canonical names or order: %v", names)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled request still active")
	}
}

func TestCandidateCDNReportsProviderServerHeaders(t *testing.T) {
	for server, want := range map[string]string{"cloudflare": "Cloudflare", "AkamaiGHost": "Akamai", "origin": "unknown"} {
		got, _ := candidateCDN("", http.Header{"Server": []string{server}})
		if got != want {
			t.Errorf("Server %q: got %q, want %q", server, got, want)
		}
	}
}
