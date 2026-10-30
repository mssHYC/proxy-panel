package service

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRealityTargetProbe(t *testing.T) {
	for _, tc := range []struct {
		name            string
		h2              bool
		maxTLS          uint16
		sni, address    string
		trusted, wantOK bool
	}{
		{"valid", true, tls.VersionTLS13, "example.com", "93.184.216.34", true, true},
		{"wrong SNI", true, tls.VersionTLS13, "wrong.invalid", "93.184.216.34", true, false},
		{"untrusted cert", true, tls.VersionTLS13, "example.com", "93.184.216.34", false, false},
		{"TLS12", true, tls.VersionTLS12, "example.com", "93.184.216.34", true, false},
		{"no h2", false, tls.VersionTLS13, "example.com", "93.184.216.34", true, false},
		{"private DNS", true, tls.VersionTLS13, "example.com", "127.0.0.1", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("CF-Ray", "test")
				w.Header().Set("Location", "https://other.invalid")
				w.WriteHeader(301)
			}))
			srv.EnableHTTP2 = tc.h2
			srv.TLS = &tls.Config{MaxVersion: tc.maxTLS}
			srv.StartTLS()
			defer srv.Close()
			roots := x509.NewCertPool()
			if tc.trusted {
				roots.AddCert(srv.Certificate())
			}
			lookups, dials := 0, 0
			result, err := checkRealityTarget(context.Background(), map[string]interface{}{"dest": "target.example:443", "server_names": []string{tc.sni}}, func(context.Context, string) ([]net.IPAddr, error) {
				lookups++
				return []net.IPAddr{{IP: net.ParseIP(tc.address)}}, nil
			}, func(ctx context.Context, network, addr string) (net.Conn, error) {
				dials++
				if addr != "93.184.216.34:443" {
					t.Fatalf("unvalidated destination dialed: %s", addr)
				}
				return (&net.Dialer{}).DialContext(ctx, network, srv.Listener.Addr().String())
			}, &tls.Config{RootCAs: roots})
			if (err == nil) != tc.wantOK {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			if lookups != 1 {
				t.Fatalf("DNS resolved %d times", lookups)
			}
			if strings.Contains(tc.name, "private") && dials != 0 {
				t.Fatal("private address dialed")
			}
			if tc.wantOK && len(result.Warnings) != 3 {
				t.Fatalf("CDN/redirect warnings missing: %+v", result)
			}
		})
	}
}

func TestRealitySaveChecksBeforePersistence(t *testing.T) {
	db := openSyncTestDB(t)
	s := NewNodeService(db, nil)
	checks := 0
	s.realityCheck = func(context.Context, map[string]interface{}) (*RealityCheck, error) {
		checks++
		return nil, errors.New("unreachable")
	}
	req := &CreateNodeReq{Name: "reality", Host: "vpn.example", Port: 443, Protocol: "vless", Transport: "tcp", Settings: `{"security":"reality","dest":"example.com:443","server_names":["example.com"]}`}
	if _, err := s.Create(req); !errors.Is(err, ErrInvalidNodeSettings) {
		t.Fatal(err)
	}
	nodes, _ := s.List()
	if len(nodes) != 0 || checks != 1 {
		t.Fatal("invalid target persisted")
	}
	s.realityCheck = func(context.Context, map[string]interface{}) (*RealityCheck, error) {
		checks++
		return &RealityCheck{}, nil
	}
	node, err := s.Create(req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Update(node.ID, &UpdateNodeReq{Settings: &req.Settings}); err != nil {
		t.Fatal(err)
	}
	if checks != 2 {
		t.Fatal("unchanged target unnecessarily probed")
	}
	changed := `{"security":"reality","dest":"other.example:443","server_names":["example.com"]}`
	s.realityCheck = func(context.Context, map[string]interface{}) (*RealityCheck, error) {
		return nil, errors.New("unreachable")
	}
	if _, err = s.Update(node.ID, &UpdateNodeReq{Settings: &changed}); !errors.Is(err, ErrInvalidNodeSettings) {
		t.Fatal(err)
	}
	saved, _ := s.GetByID(node.ID)
	if saved.Settings != req.Settings {
		t.Fatal("failed target update persisted")
	}
	disabled := false
	if _, err = s.Update(node.ID, &UpdateNodeReq{Enable: &disabled}); err != nil {
		t.Fatal(err)
	}
}
