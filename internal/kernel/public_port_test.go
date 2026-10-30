package kernel

import "testing"

func TestManagedVMessRequiresLocalTLSWebTransport(t *testing.T) {
	for _, test := range []struct {
		name, protocol, transport, listen, security string
		port                                        interface{}
		valid                                       bool
	}{
		{"ws", "vmess", "ws", "127.0.0.1", "tls", float64(443), true},
		{"upgrade", "vmess", "httpupgrade", "127.0.0.1", "tls", float64(443), true},
		{"public backend", "vmess", "ws", "0.0.0.0", "tls", float64(443), false},
		{"plaintext", "vmess", "ws", "127.0.0.1", "none", float64(443), false},
		{"tcp", "vmess", "tcp", "127.0.0.1", "tls", float64(443), false},
		{"other protocol", "vless", "ws", "127.0.0.1", "tls", float64(443), false},
		{"fractional", "vmess", "ws", "127.0.0.1", "tls", 443.5, false},
		{"wrong public port", "vmess", "ws", "127.0.0.1", "tls", float64(8443), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			node := NodeConfig{Protocol: test.protocol, Transport: test.transport, Port: 18080, Settings: map[string]interface{}{
				"listen": test.listen, "security": test.security, "public_port": test.port,
			}}
			if err := ValidateNodeSecurity(node, "xray"); (err == nil) != test.valid {
				t.Fatalf("valid=%v, error=%v", test.valid, err)
			}
		})
	}
}
