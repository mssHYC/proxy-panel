package model

import "testing"

func TestPublicPortKeepsUnmanagedEndpoints(t *testing.T) {
	for _, tc := range []struct {
		settings string
		want     int
	}{
		{`{"public_port":443,"security":"tls","listen":"127.0.0.1"}`, 443},
		{`{"public_port":443,"tls":true,"listen":"127.0.0.1"}`, 443},
		{`{"security":"tls"}`, 18080},
		{`{"public_port":443,"security":"tls","listen":"0.0.0.0"}`, 18080},
		{`{"public_port":443,"security":"none","listen":"127.0.0.1"}`, 18080},
		{`{"public_port":8443,"security":"tls","listen":"127.0.0.1"}`, 18080},
		{`{"public_port":443.5}`, 18080},
		{`invalid`, 18080},
	} {
		n := Node{Protocol: "vmess", Transport: "ws", Port: 18080, Settings: tc.settings}
		if got := n.PublicPort(); got != tc.want {
			t.Fatalf("%s: got %d, want %d", tc.settings, got, tc.want)
		}
		n.Protocol = "vless"
		if got := n.PublicPort(); got != n.Port {
			t.Fatalf("changed unrelated protocol port: %d", got)
		}
	}
}
