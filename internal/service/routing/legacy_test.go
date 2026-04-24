package routing

import (
	"reflect"
	"testing"
)

func TestParseLegacyRules_AllTypes(t *testing.T) {
	text := `
# comment line
DOMAIN,example.com,Google
DOMAIN-SUFFIX,gmail.com,Google
DOMAIN-KEYWORD,youtube,YouTube
IP-CIDR,1.1.1.1/32,本地直连
IP-CIDR6,2001::/64,本地直连
GEOSITE,cn,本地直连
GEOIP,cn,本地直连
PROCESS-NAME,curl,DIRECT

`
	rules, err := ParseLegacyRules(text)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(rules) != 8 {
		t.Fatalf("expected 8 rules, got %d", len(rules))
	}
	if rules[0].Type != "DOMAIN" || rules[0].Value != "example.com" || rules[0].Outbound != "Google" {
		t.Fatalf("rule[0] = %+v", rules[0])
	}
}

func TestParseLegacyRules_IgnoreBlankAndComments(t *testing.T) {
	text := "# hello\n\n  \n"
	rules, err := ParseLegacyRules(text)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(rules) != 0 {
		t.Fatalf("expected 0, got %+v", rules)
	}
}

func TestParseLegacyRules_MalformedSkipped(t *testing.T) {
	text := "NOT-A-RULE\nDOMAIN-SUFFIX,foo.com\nDOMAIN-SUFFIX,foo.com,X"
	rules, err := ParseLegacyRules(text)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !reflect.DeepEqual(rules, []LegacyRule{{Type: "DOMAIN-SUFFIX", Value: "foo.com", Outbound: "X"}}) {
		t.Fatalf("got %+v", rules)
	}
}

func TestMapLegacyOutboundToCode(t *testing.T) {
	cases := map[string]string{
		"手动切换": "node_select",
		"自动选择": "auto_select",
		"Google":  "google",
		"DIRECT":  "DIRECT",
		"REJECT":  "REJECT",
		"漏网之鱼": "fallback",
		"Unknown": "",
	}
	for in, want := range cases {
		if got := MapLegacyOutboundToCode(in); got != want {
			t.Errorf("MapLegacyOutboundToCode(%q) = %q, want %q", in, got, want)
		}
	}
}
