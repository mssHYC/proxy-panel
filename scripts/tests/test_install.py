import os
from pathlib import Path
import subprocess
import tempfile
import unittest


INSTALLER = Path(__file__).resolve().parents[1] / "install.sh"


class InstallTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        (self.root / "config.yaml").write_text(
            'server:\n  listen: "127.0.0.1"\n  port: 8081\n'
            '  tls: true\n  domain: "panel.example.com"\n'
            'auth:\n  admin_pass: "do-not-print-this"\n'
        )
        binary = self.root / "proxy-panel"
        binary.write_text('''#!/bin/bash
echo health >> "$TEST_ROOT/events"
[[ "${HEALTH_EXIT:-0}" == 0 ]] || echo "TLS health probe failed" >&2
exit "${HEALTH_EXIT:-0}"
''')
        binary.chmod(0o755)

    def run_shell(self, body, **env):
        return subprocess.run(
            ["bash", "-c", '''
source "$INSTALLER"
INSTALL_DIR="$TEST_ROOT"
CONFIG_FILE="$TEST_ROOT/config.yaml"
DB_FILE="$TEST_ROOT/panel.db"
WEBSITE_CONFIG_DIR="$TEST_ROOT/website"
check_root() { :; }
sleep() { :; }
if [[ "$(uname)" == Darwin ]]; then
    sed() {
        if [[ "$1" == -i ]]; then shift; command sed -i '' "$@"; else command sed "$@"; fi
    }
fi
curl() { echo 192.0.2.10; }
systemctl() { echo "systemctl $*" >> "$TEST_ROOT/events"; }
journalctl() { echo "panel failure details"; }
''' + body],
            env={**os.environ, "INSTALLER": str(INSTALLER),
                 "TEST_ROOT": str(self.root), **env},
            capture_output=True, text=True, timeout=15,
        )

    def events(self):
        path = self.root / "events"
        return path.read_text() if path.exists() else ""

    def test_service_start_failure_is_not_success(self):
        result = self.run_shell('''
systemctl() {
    [[ "$*" != "start proxy-panel" ]] || { echo "cannot start panel" >&2; return 1; }
}
start_services
''')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("cannot start panel", result.stderr)
        self.assertNotIn("服务启动完成", result.stdout)

    def test_active_process_without_healthy_listener_is_not_success(self):
        result = self.run_shell("start_services", HEALTH_EXIT="1")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("panel failure details", result.stdout + result.stderr)
        self.assertIn("TLS health probe failed", result.stderr)
        self.assertNotIn("服务启动完成", result.stdout)

    def test_exited_process_is_not_success_even_if_health_probe_would_pass(self):
        result = self.run_shell('''
systemctl() { [[ "$1" != is-active ]]; }
start_services
''')
        self.assertNotEqual(result.returncode, 0)

    def test_healthy_panel_passes_startup(self):
        result = self.run_shell("start_services")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("health", self.events())

    def test_delayed_listener_is_allowed_to_become_ready(self):
        (self.root / "proxy-panel").write_text('''#!/bin/bash
echo health >> "$TEST_ROOT/events"
[[ $(grep -c '^health$' "$TEST_ROOT/events") -ge 3 ]]
''')
        result = self.run_shell("start_services")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.events().count("health\n"), 3)

    def install_flow(self, failure):
        (self.root / "proxy-panel").unlink()
        return self.run_shell('''
for stage in detect_os detect_arch install_deps interactive_config setup_firewall \
    generate_config generate_default_xray_config generate_default_singbox_config \
    download_xray download_singbox download_panel install_cli setup_systemd start_services; do
    eval "$stage() { :; }"
done
print_summary() { echo INSTALL_SUCCESS; }
offer_website_setup() { echo WEBSITE_OFFER; }
''' + failure + "\ndo_install\n")

    def test_install_does_not_continue_after_failed_startup(self):
        result = self.install_flow("start_services() { return 1; }")
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn("INSTALL_SUCCESS", result.stdout)
        self.assertNotIn("WEBSITE_OFFER", result.stdout)

    def test_install_does_not_continue_after_failed_configuration(self):
        result = self.install_flow("interactive_config() { return 1; }")
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn("INSTALL_SUCCESS", result.stdout)

    def test_install_does_not_continue_after_failed_download(self):
        result = self.install_flow("download_panel() { return 1; }")
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn("INSTALL_SUCCESS", result.stdout)

    def test_missing_release_is_a_download_failure(self):
        result = self.run_shell('curl() { echo \'{"tag_name":null}\'; }; download_panel')
        self.assertNotEqual(result.returncode, 0)

    def test_dependency_installation_includes_acme_requirements(self):
        result = self.run_shell('''
PKG_MGR=apt
apt() { echo "apt $*" >> "$TEST_ROOT/events"; }
install_deps
''')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("cron", self.events())
        self.assertIn("openssl", self.events())
        self.assertIn("ca-certificates", self.events())

    def test_failed_package_install_is_not_success(self):
        result = self.run_shell('''
PKG_MGR=apt
apt() { echo "package error" >&2; return 1; }
install_deps
''')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("package error", result.stderr)
        self.assertNotIn("依赖安装完成", result.stdout)

    def test_tls_summary_uses_domain_and_explains_loopback_access(self):
        result = self.run_shell('''
PANEL_PORT=8081 TLS_ENABLED=true DOMAIN=panel.example.com ADMIN_USER=admin
print_summary
''')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("https://panel.example.com:8081", result.stdout)
        self.assertNotIn("https://127.0.0.1:8081", result.stdout)
        self.assertIn("127.0.0.1 panel.example.com", result.stdout)
        self.assertIn("443", result.stdout)
        self.assertIn("Cloudflare", result.stdout)
        self.assertNotIn("do-not-print-this", result.stdout)

    def test_http_summary_uses_loopback_without_hosts_edit(self):
        (self.root / "config.yaml").write_text(
            'server:\n  listen: "127.0.0.1"\n  port: 8081\n  tls: false\n  domain: ""\n'
        )
        result = self.run_shell('PANEL_PORT=8081 TLS_ENABLED=false DOMAIN="" ADMIN_USER=admin; print_summary')
        self.assertIn("http://127.0.0.1:8081", result.stdout)
        self.assertNotIn("hosts", result.stdout)

    def test_restart_does_not_report_success_for_unhealthy_panel(self):
        result = self.run_shell("do_restart", HEALTH_EXIT="1")
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn("服务重启完成", result.stdout)

    def test_port_change_does_not_report_success_for_unhealthy_panel(self):
        result = self.run_shell("do_port port 8443", HEALTH_EXIT="1")
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn("端口已改为", result.stdout)

    def test_port_change_explains_retained_loopback_binding(self):
        result = self.run_shell("do_port port 8443")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("127.0.0.1:8443", result.stdout)
        self.assertIn("仅本机", result.stdout)
        self.assertIn("https://panel.example.com:8443/", result.stdout)
        self.assertIn('listen: "127.0.0.1"', (self.root / "config.yaml").read_text())

    def test_failed_port_write_does_not_restart_or_claim_success(self):
        result = self.run_shell('sed() { echo "write failed" >&2; return 1; }; do_port port 8443')
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn("restart", self.events())
        self.assertNotIn("端口已改为", result.stdout)


if __name__ == "__main__":
    unittest.main()
