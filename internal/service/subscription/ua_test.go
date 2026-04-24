package subscription

import "testing"

func TestSniffFormat(t *testing.T) {
	cases := []struct {
		ua   string
		want string
	}{
		{"Surge iOS/2589", "surge"},
		{"Shadowrocket/1993", "shadowrocket"},
		{"Quantumult X", "shadowrocket"},
		{"ClashX Pro/1.95", "clash"},
		{"Clash/1.0", "clash"},
		{"Stash/2.6", "clash"},
		{"mihomo/1.18", "clash"},
		{"sing-box 1.9.3", "singbox"},
		{"SingBox/1.9", "singbox"},
		{"v2rayN/6.30", "v2ray"},
		{"V2Box 2.1", "v2ray"},
		{"Mozilla/5.0 (Macintosh)", ""},
		{"", ""},
		{"curl/8.6.0", ""},
	}
	for _, c := range cases {
		got := SniffFormat(c.ua)
		if got != c.want {
			t.Errorf("SniffFormat(%q) = %q, want %q", c.ua, got, c.want)
		}
	}
}
