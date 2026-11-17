"""Fault injection for certificate setup without network, root or real services."""
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

INSTALLER = Path(__file__).resolve().parents[1] / "install.sh"


class TLSInstallTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        # Remap only the fixed ACME filesystem boundary; never touch the user's home.
        self.installer = self.root / "install.sh"
        self.installer.write_text(INSTALLER.read_text().replace("~/.acme.sh", '"$TEST_ROOT/.acme.sh"'))
        (self.root / "certs").mkdir()
        (self.root / "bin").mkdir()
        self.config = self.root / "config.yaml"
        self.original_config = ('server:\n  listen: "127.0.0.1"\n  port: 8081\n'
                                '  tls: false\n  cert: "old.crt"\n  key: "old.key"\n'
                                '  domain: "old.example.com"\n')
        self.config.write_text(self.original_config)
        self.fake_acme = self.root / "fake-acme"
        self.fake_acme.write_text('''#!/bin/bash
echo "$1" >> "$TEST_ROOT/acme-events"
case "$1" in
 --version) [[ "$FAIL" != version ]] || { echo 'acme cannot execute' >&2; exit 1; }; echo v-test;;
 --set-default-ca) [[ "$FAIL" != ca ]] || { echo 'CA configuration failed' >&2; exit 1; };;
 --issue) [[ "$FAIL" != issue ]] || { echo 'ACME challenge rejected' >&2; exit 1; };;
 --install-cert)
  [[ "$FAIL" != deploy ]] || { echo 'certificate copy failed' >&2; exit 1; }
  while [[ $# -gt 0 ]]; do
   case "$1" in
    --fullchain-file) cert="$2"; shift;;
    --key-file) key="$2"; shift;;
    --reloadcmd) reload="$2"; shift;;
   esac
   shift
  done
  if [[ "$FAIL" != missing-cert ]]; then printf 'certificate\n' > "$cert"; fi
  if [[ "$FAIL" != missing-key ]]; then printf 'private-key\n' > "$key"; fi
  if [[ "$FAIL" == empty-cert ]]; then : > "$cert"; fi
  sh -c "$reload";;
esac
''')
        self.fake_acme.chmod(0o755)
        systemctl = self.root / "bin/systemctl"
        systemctl.write_text('''#!/bin/bash
echo "$*" >> "$TEST_ROOT/service-events"
case "$1" in
 is-active) [[ "$ACTIVE" == yes ]];;
 restart) [[ "$ACTIVE" == yes && "$FAIL" != reload ]];;
esac
''')
        systemctl.chmod(0o755)

    def install_fake_acme(self, executable=True):
        directory = self.root / ".acme.sh"
        directory.mkdir(exist_ok=True)
        path = directory / "acme.sh"
        path.write_bytes(self.fake_acme.read_bytes())
        path.chmod(0o755 if executable else 0o644)

    def run_shell(self, body, input="", fail="", active="no"):
        compatibility = ""
        if sys.platform == "darwin":
            compatibility = '''sed() {
 if [[ "$1" == -i ]]; then shift; command sed -i '' "$@"; else command sed "$@"; fi
}
'''
        script = '''source "$INSTALLER"
INSTALL_DIR="$TEST_ROOT"
CONFIG_FILE="$TEST_ROOT/config.yaml"
DOMAIN=site.example.com
CERT_PATH="$TEST_ROOT/certs/site.example.com.crt"
KEY_PATH="$TEST_ROOT/certs/site.example.com.key"
check_root() { :; }
cert_status() { :; }
ss() { :; }
stop_all_services() { echo stop >> "$TEST_ROOT/service-events"; }
start_services() { echo start >> "$TEST_ROOT/service-events"; [[ "$FAIL" != start ]]; }
''' + compatibility + body
        return subprocess.run(["bash", "-c", script], input=input, text=True,
                              stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=10,
                              env={**os.environ, "TEST_ROOT": str(self.root),
                                   "INSTALLER": str(self.installer), "FAIL": fail, "ACTIVE": active,
                                   "PATH": str(self.root / "bin") + os.pathsep + os.environ["PATH"]})

    def events(self, name):
        path = self.root / name
        return path.read_text() if path.exists() else ""

    def inputs(self, mode, wildcard=False):
        domain = "*.example.com" if wildcard else "site.example.com"
        return {1: f"1\n{domain}\n", 2: f"2\n{domain}\nfixture-token\nzone-id\n",
                3: f"3\n{domain}\nfixture-id\nfixture-token\n",
                4: f"4\n{domain}\nfixture-id\nfixture-secret\n"}[mode]

    def test_zero_exit_bootstrap_without_acme_is_rejected_and_keeps_error(self):
        result = self.run_shell('''curl() {
 printf '%s\n' 'echo "missing crontab" >&2' 'echo "Install error"' 'exit 0'
}
install_acme
''')
        self.assertNotEqual(result.returncode, 0, result.stdout)
        self.assertIn("missing crontab", result.stdout)

    def test_bootstrap_download_failure_is_not_lost_in_pipeline(self):
        result = self.run_shell('curl() { echo "download failed" >&2; return 22; }; install_acme')
        self.assertNotEqual(result.returncode, 0, result.stdout)
        self.assertIn("download failed", result.stdout)

    def test_failed_bootstrap_cannot_reach_cloudflare_or_modify_configuration(self):
        result = self.run_shell('''curl() {
 printf '%s\n' 'echo "missing crontab" >&2' 'echo "Install error"' 'exit 0'
}
cert_setup
''', self.inputs(2))
        self.assertNotEqual(result.returncode, 0, result.stdout)
        self.assertIn("missing crontab", result.stdout)
        self.assertNotIn("正在通过 Cloudflare", result.stdout)
        self.assertNotIn("证书申请失败，请检查 API Token", result.stdout)
        self.assertNotIn("证书设置完成", result.stdout)
        self.assertEqual(self.config.read_text(), self.original_config)
        self.assertEqual(self.events("service-events"), "")

    def test_successful_bootstrap_is_checked_and_selects_ca(self):
        result = self.run_shell('''curl() {
 printf '%s\n' 'mkdir -p "$TEST_ROOT/.acme.sh"' 'cp "$TEST_ROOT/fake-acme" "$TEST_ROOT/.acme.sh/acme.sh"' 'chmod 755 "$TEST_ROOT/.acme.sh/acme.sh"'
}
install_acme
''')
        self.assertEqual(result.returncode, 0, result.stdout)
        self.assertEqual(self.events("acme-events").splitlines(), ["--version", "--set-default-ca"])

    def test_unusable_acme_or_ca_failure_is_rejected(self):
        for fail in ("version", "ca", "not-executable"):
            with self.subTest(fail=fail):
                self.install_fake_acme(executable=fail != "not-executable")
                result = self.run_shell("install_acme", fail=fail)
                self.assertNotEqual(result.returncode, 0, result.stdout)

    def test_acme_failure_stops_all_certificate_modes(self):
        for mode in (1, 2, 3, 4):
            with self.subTest(mode=mode):
                result = self.run_shell('install_acme() { return 1; }; setup_tls', self.inputs(mode))
                self.assertNotEqual(result.returncode, 0, result.stdout)
                self.assertNotIn("正在申请证书", result.stdout)
                self.assertNotIn("正在通过", result.stdout)

    def test_issue_failure_stops_all_certificate_modes(self):
        self.install_fake_acme()
        for mode, wildcard in ((1, False), (2, False), (2, True), (3, False), (4, False)):
            with self.subTest(mode=mode, wildcard=wildcard):
                result = self.run_shell("setup_tls", self.inputs(mode, wildcard), fail="issue")
                self.assertNotEqual(result.returncode, 0, result.stdout)
                self.assertIn("ACME challenge rejected", result.stdout)
                self.assertNotIn("证书申请成功", result.stdout)
                self.assertNotIn("--install-cert", self.events("acme-events"))

    def test_certificate_deployment_failures_do_not_report_success(self):
        self.install_fake_acme()
        for fail in ("deploy", "missing-cert", "missing-key", "empty-cert", "reload"):
            with self.subTest(fail=fail):
                for file in (self.root / "certs").iterdir():
                    file.unlink()
                result = self.run_shell('install_cert ecc', fail=fail, active="yes")
                self.assertNotEqual(result.returncode, 0, result.stdout)
                self.assertNotIn("证书已安装到", result.stdout)
                if fail == "deploy":
                    self.assertIn("certificate copy failed", result.stdout)

    def test_initial_install_does_not_restart_services_that_are_not_running(self):
        self.install_fake_acme()
        result = self.run_shell("install_cert ecc")
        self.assertEqual(result.returncode, 0, result.stdout)
        self.assertNotIn("restart", self.events("service-events"))
        self.assertIn("证书已安装到", result.stdout)

    def test_certificate_failure_preserves_config_and_services(self):
        self.install_fake_acme()
        for fail in ("issue", "deploy", "missing-key"):
            with self.subTest(fail=fail):
                (self.root / "service-events").unlink(missing_ok=True)
                for file in (self.root / "certs").iterdir():
                    file.unlink()
                result = self.run_shell("cert_setup", self.inputs(2), fail=fail)
                self.assertNotEqual(result.returncode, 0, result.stdout)
                self.assertEqual(self.config.read_text(), self.original_config)
                self.assertNotIn("\nstop\n", "\n" + self.events("service-events"))
                self.assertNotIn("\nstart\n", "\n" + self.events("service-events"))
                self.assertNotIn("config.yaml 已更新", result.stdout)
                self.assertNotIn("证书设置完成", result.stdout)

    def test_cert_setup_propagates_service_failure(self):
        self.install_fake_acme()
        result = self.run_shell("cert_setup", self.inputs(2), fail="start")
        self.assertNotEqual(result.returncode, 0, result.stdout)
        self.assertNotIn("证书设置完成", result.stdout)

    def test_successful_cert_setup_updates_config_and_starts_services(self):
        self.install_fake_acme()
        result = self.run_shell("cert_setup", self.inputs(2))
        self.assertEqual(result.returncode, 0, result.stdout)
        self.assertIn('  tls: true', self.config.read_text())
        self.assertIn('  domain: "site.example.com"', self.config.read_text())
        self.assertIn("\nstop\nstart\n", "\n" + self.events("service-events"))
        self.assertIn("证书设置完成", result.stdout)


if __name__ == "__main__":
    unittest.main()
