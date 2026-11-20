package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"proxy-panel/internal/config"
	"proxy-panel/internal/service/firewall"
	"proxy-panel/internal/website"
)

func recordingNodeFirewall(t *testing.T) (*NodeService, func() string) {
	t.Helper()
	dir := t.TempDir()
	logfile := filepath.Join(dir, "rules")
	if err := os.WriteFile(logfile, nil, 0600); err != nil {
		t.Fatal(err)
	}
	cmd := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$TEST_FIREWALL_LOG\"\n"
	if err := os.WriteFile(filepath.Join(dir, "ufw"), []byte(cmd), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TEST_FIREWALL_LOG", logfile)
	fw, err := firewall.NewService(config.FirewallConfig{Enable: true, Backend: "ufw"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return NewNodeService(openSyncTestDB(t), fw), func() string {
		t.Helper()
		b, err := os.ReadFile(logfile)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
}

func TestFirewallUsesCurrentOwnersForQueuedPortChanges(t *testing.T) {
	s, recorded := recordingNodeFirewall(t)
	_ = seedNode(t, s.db, "singbox", "tuic") // UDP is still using the shared port.
	s.applyFirewallPort(443, revokePort)     // An older TCP-node removal runs late.
	if calls := recorded(); calls != "" {
		t.Fatalf("shared port received firewall commands: %q", calls)
	}
	if _, err := s.db.Exec("DELETE FROM nodes WHERE port=443"); err != nil {
		t.Fatal(err)
	}
	s.applyFirewallPort(443, revokePort)
	s.applyFirewallPort(443, allowPort) // A queued create came in before the last delete.
	s.EnsureFirewallPorts([]int{443})   // A stale startup snapshot must not reopen it.
	if want := "delete allow 443/tcp\ndelete allow 443/udp\n"; recorded() != want {
		t.Fatalf("expected only final port removal, got %q", recorded())
	}

	_ = seedNode(t, s.db, "xray", "vless")
	s.EnsureFirewallPorts([]int{443})
	if !strings.HasSuffix(recorded(), "allow 443/tcp\nallow 443/udp\n") {
		t.Fatalf("enabled port did not reopen: %q", recorded())
	}
}

func TestFirewallPreservesManagedWebsiteRuleWithoutEnabledNodes(t *testing.T) {
	s, recorded := recordingNodeFirewall(t)
	manifest := filepath.Join(t.TempDir(), "managed.json")
	s.website = website.New(manifest, "", "", "")
	for _, content := range []string{
		`{"mode":"standalone","listens":["192.0.2.1:443"]}`,
		`{"mode":"proxy","node_id":7,"listens":["192.0.2.1:443"]}`,
		`{broken`,
	} {
		if err := os.WriteFile(manifest, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		s.applyFirewallPort(443, revokePort)
		s.EnsureFirewallPorts([]int{443})
		if got := recorded(); got != "" {
			t.Fatalf("unowned rule should be preserved, got %q", got)
		}
	}
}
