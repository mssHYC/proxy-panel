package service_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagedRealitySELinuxBindPreflight(t *testing.T) {
	f := newManagedRealityFixture(t, "0.0.0.0")
	script, err := filepath.Abs("../../scripts/install.sh")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEST_MANAGEMENT_SCRIPT", script)
	t.Setenv("TEST_SELINUX_MODE", "Enforcing")
	logfile := filepath.Join(f.root, "service-commands")
	t.Setenv("TEST_SERVICE_LOG", logfile)
	if err := os.Rename(filepath.Join(f.root, "systemctl"), filepath.Join(f.root, "fixture-systemctl")); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"getenforce": `echo "$TEST_SELINUX_MODE"`,
		// Use the actual management CLI from this checkout and the fixture's site directory.
		"bash": `[ "$1" = /usr/local/bin/proxy-panel ] || exit 1
shift
exec /bin/bash -c 'source "$TEST_MANAGEMENT_SCRIPT"; WEBSITE_CONFIG_DIR="$NODE_WEBSITE_TEST_ROOT"; main "$@"' cli "$@"`,
		"systemctl": `echo "$*" >> "$TEST_SERVICE_LOG"
if [ "$1" = show ]; then echo SELinuxContext=test_nginx_context; exit 0; fi
exec "$NODE_WEBSITE_TEST_ROOT/fixture-systemctl" "$@"`,
		"chcon":      "exit 0",
		"journalctl": "exit 0",
		"systemd-run": `echo "$*" >> "$TEST_SERVICE_LOG"
while [ "$1" != "$NODE_WEBSITE_TEST_ROOT/nginx" ]; do [ "$#" -gt 1 ] || exit 2; shift; done
TEST_NGINX_UNIT=1 exec "$@"`,
		"nginx": `if [ "$TEST_NGINX_UNIT" = 1 ]; then
    [ "$1" = -t ] && [ "$2" = -c ] || exit 2
    cat "$3" >> "$TEST_SERVICE_LOG"
    echo "simulated name_bind denial" >> "$TEST_SERVICE_LOG"
    exit 1
fi`,
	} {
		if err := os.WriteFile(filepath.Join(f.root, name), []byte("#!/bin/sh\n"+body+"\n"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string][]byte{}
	for _, name := range []string{"managed.json", "nginx.conf", "active-nginx.conf", "xray.json", "active-xray.json"} {
		files[name] = f.read(t, name)
	}
	f.put(t, map[string]any{"port": 443}, 409)
	f.verify(t, f.port, "0.0.0.0", "a.example.com")
	for name, before := range files {
		if !bytes.Equal(before, f.read(t, name)) {
			t.Errorf("SELinux bind denial changed %s", name)
		}
	}
	commands := string(f.read(t, "service-commands"))
	for _, expected := range []string{"SELinuxContext=test_nginx_context", "listen 127.0.0.1:9443;", "simulated name_bind denial", "reset-failed proxy-panel-web-bind-"} {
		if !strings.Contains(commands, expected) {
			t.Fatalf("missing %q in probe commands: %s", expected, commands)
		}
	}
	if strings.Contains(commands, "restart") || strings.Contains(commands, "reload") {
		t.Fatalf("denied save switched a service: %s", commands)
	}
	if matches, err := filepath.Glob(filepath.Join(f.root, ".bind-check.*")); err != nil || len(matches) != 0 {
		t.Fatalf("leftover probes: %v (%v)", matches, err)
	}

	t.Setenv("TEST_SELINUX_MODE", "Permissive")
	f.put(t, map[string]any{"port": 443}, 200)
	shared := f.verify(t, 443, "127.0.0.2", "a.example.com")
	t.Setenv("TEST_SELINUX_MODE", "Enforcing")
	settings := strings.ReplaceAll(shared.Settings, "a.example.com", "b.example.com")
	f.put(t, map[string]any{"settings": settings}, 200)
	f.put(t, map[string]any{"port": f.port}, 200)
	f.verify(t, f.port, "0.0.0.0", "b.example.com")
	if strings.Count(string(f.read(t, "service-commands")), "simulated name_bind denial") != 1 {
		t.Fatal("a save with no new enforcing-mode Nginx listener repeated the bind probe")
	}
}
