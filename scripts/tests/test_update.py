"""Fault injection for the real installer flow; no root/network/services needed."""
import json
import os
import pty
import select
from pathlib import Path
import shutil
import subprocess
import tarfile
import tempfile
import time
import unittest

INSTALLER = Path(__file__).resolve().parents[1] / "install.sh"

class PromptTests(unittest.TestCase):
    def test_no_controlling_terminal_uses_defaults_without_reading_stdin(self):
        script = '''source "$INSTALLER"
for prompt in Xray Singbox Website; do
 if confirm "$prompt"; then exit 4; fi
done
confirm_default_yes default_yes || exit 5
read -r leftover
[[ "$leftover" == y ]] || exit 6
'''
        result = subprocess.run(['bash', '-c', script], env={**os.environ, 'INSTALLER': str(INSTALLER)},
                                input='y\n', text=True, capture_output=True, start_new_session=True, timeout=5)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(result.stderr, '')
        self.assertEqual(result.stdout.count('非交互环境，采用默认值'), 4)

    def test_pipe_install_still_reads_answer_from_controlling_terminal(self):
        pid, fd = pty.fork()
        if pid == 0:
            os.execve('/bin/bash', ['bash', '-c', '''source "$INSTALLER"
printf 'n\\n' | { if confirm terminal; then echo ANSWER=yes; else echo ANSWER=no; fi; }
'''], {**os.environ, 'INSTALLER': str(INSTALLER)})
        output = b''
        answered = False
        try:
            deadline = time.monotonic() + 5
            while time.monotonic() < deadline:
                if not select.select([fd], [], [], .1)[0]: continue
                try: chunk = os.read(fd, 4096)
                except OSError: break
                if not chunk: break
                output += chunk
                if b'[y/N]:' in output and not answered:
                    os.write(fd, b'y\n')
                    answered = True
                if b'ANSWER=' in output: break
            self.assertIn(b'ANSWER=yes', output)
            self.assertNotIn('非交互环境'.encode(), output)
        finally:
            os.close(fd)
            os.waitpid(pid, 0)

class UpdateTests(unittest.TestCase):
    def run_case(self, failure="", active=True, without_cmp=False, existing_access=False, tag=None, rollback=False):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            (root / "data").mkdir()
            (root / "states").mkdir()
            originals = {"proxy-panel": b"old-binary", "config.yaml": b"listen: 0.0.0.0", "data/panel.db": b"old-db", "xray.json": b"old-config", "xray": b"old-core"}
            if failure == "pending-website":
                (root / ".website-transaction").symlink_to(root / "website-backup")
            cli_name = "bin-managed/proxy-panel"
            originals[cli_name] = b"#!/bin/sh\necho old-cli\n"
            access_name = "units/proxy-panel.service.d/managed-website.conf"
            if existing_access:
                originals[access_name] = b"[Service]\nReadWritePaths=-/old-managed-directory\n"
            for name, data in originals.items():
                (root / name).parent.mkdir(parents=True, exist_ok=True)
                (root / name).write_bytes(data)
            services = ["proxy-panel", "xray"] if active else []
            for svc in services:
                (root / "states" / svc).touch()
            candidate = root / "candidate"
            candidate.write_text('''#!/bin/bash
case "$1" in
-compatibility) echo '{"xray":"v26.10.31","sing-box":"v1.14.4"}'; exit;;
esac
for ((i=1;i<=$#;i++)); do
 if [[ "${!i}" == -check-update ]]; then
  [[ "$FAIL" != preflight ]] || exit 2
  [[ "$FAIL" != final-preflight || "${@}" != *final-check* ]] || exit 2
  j=$((i+1)); dir="${!j}"; mkdir -p "$dir"
  cp "$TEST_ROOT/manifest.json" "$dir/manifest.json"
  if [[ "$dir" == */final-check ]]; then
   case "$FAIL" in
    manifest-change) jq '.xray_path += ".changed"' "$TEST_ROOT/manifest.json" > "$dir/manifest.json";;
    manifest-invalid) echo '{' > "$dir/manifest.json";;
    manifest-missing) rm "$dir/manifest.json";;
   esac
  fi
  exit 0
 fi
 if [[ "${!i}" == -check-health ]]; then
  [[ "$FAIL" != health ]] || exit 2
  exit 0
 fi
done
exit 1
''')
            candidate.chmod(0o755)
            with tarfile.open(root / "release.tar.gz", "w:gz") as archive:
                archive.add(candidate, arcname="proxy-panel")
                if failure != "missing-cli":
                    archive.add(INSTALLER, arcname="scripts/install.sh")
            manifest = {"files": [str(root / name) for name in originals if name not in ("proxy-panel", cli_name)] + [str(root / "data/panel.db-wal")], "xray_path":str(root / "xray"), "singbox_path":str(root / "sing-box")}
            (root / "manifest.json").write_text(json.dumps(manifest))
            script = root / "run.sh"
            script.write_text('''source "$INSTALLER"
INSTALL_DIR="$TEST_ROOT"
CONFIG_FILE="$TEST_ROOT/config.yaml"
BACKUP_DIR="$TEST_ROOT/backups"
CLI_PATH="$TEST_ROOT/bin-managed/proxy-panel"
WEBSITE_UNIT_DIR="$TEST_ROOT/units"
WEBSITE_CONFIG_DIR="$TEST_ROOT/website"
ARCH=amd64
flock() { :; }
sleep() { :; }
confirm() { return 1; }
replace_update_binary() {
 cp "$1" "$2" && chmod 755 "$2" || return 1
 [[ "$FAIL" != replace ]]
}
fetch_update_asset() {
 echo "$2" > "$TEST_ROOT/fetched-release"
 [[ "$FAIL" != download ]] || return 1
 cp "$TEST_ROOT/release.tar.gz" "$4"
}
systemctl() {
 local op="$1" svc="${@: -1}"
 echo "$op $svc" >> "$TEST_ROOT/events"
 case "$op" in
 daemon-reload)
  if [[ "$FAIL" == daemon-reload && ! -f "$TEST_ROOT/failed-once" ]]; then touch "$TEST_ROOT/failed-once"; return 1; fi;;
 show) if [[ "$svc" == sing-box ]]; then echo not-found; else echo loaded; fi;;
 is-active) [[ -f "$TEST_ROOT/states/$svc" ]];;
 stop)
  if [[ "$FAIL" == stop && ! -f "$TEST_ROOT/failed-once" ]]; then touch "$TEST_ROOT/failed-once"; return 1; fi
  rm -f "$TEST_ROOT/states/$svc";;
 start)
  if [[ "$FAIL" == start && ! -f "$TEST_ROOT/failed-once" ]]; then touch "$TEST_ROOT/failed-once"; return 1; fi
  touch "$TEST_ROOT/states/$svc"
  if [[ "$svc" == proxy-panel ]] && ! grep -q old-binary "$TEST_ROOT/proxy-panel"; then
   echo migrated > "$TEST_ROOT/data/panel.db"
   echo candidate-wal > "$TEST_ROOT/data/panel.db-wal"
   echo generated > "$TEST_ROOT/xray.json"
  fi;;
 esac
}
if [[ -n "$REQUESTED_TAG" ]]; then
 check_root() { :; }
 detect_os() { :; }
 detect_arch() { :; }
 offer_website_setup() { :; }
 main update "$REQUESTED_TAG"
else
 run_safe_update
fi
rc=$?
if [[ "$rc" == 0 && "$ROLLBACK" == 1 ]]; then
 check_root() { :; }
 confirm() { return 0; }
 do_rollback_update "$BACKUP_DIR"/update-*
else
 exit "$rc"
fi
''')
            env = {**os.environ, "TEST_ROOT":temp,"INSTALLER":str(INSTALLER),"FAIL":failure,
                   "REQUESTED_TAG": tag or "", "ROLLBACK": str(int(rollback))}
            if without_cmp:
                bin_dir = root / "bin"
                bin_dir.mkdir()
                for command in ("bash", "chmod", "cp", "date", "grep", "gzip", "jq", "mkdir", "mktemp", "mv", "rm", "tar", "touch"):
                    executable = shutil.which(command)
                    self.assertIsNotNone(executable, command)
                    (bin_dir / command).symlink_to(executable)
                env["PATH"] = str(bin_dir)
            result = subprocess.run(["bash",str(script)],env=env,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True)
            self.assertEqual(result.returncode == 0, not failure, result.stdout)
            release = root / "fetched-release"
            if failure in ("invalid-tag", "pending-website"):
                self.assertFalse(release.exists())
            else:
                self.assertEqual(release.read_text().strip(), "tags/" + tag if tag else "latest")
            if failure or rollback:
                for name, data in originals.items():
                    self.assertEqual((root / name).read_bytes(), data, name + result.stdout)
                self.assertFalse((root / "data/panel.db-wal").exists(),result.stdout)
                if not existing_access:
                    self.assertFalse((root / access_name).exists(), "failed upgrade left new permissions")
            else:
                self.assertEqual((root / "config.yaml").read_bytes(),originals["config.yaml"])
                self.assertEqual((root / "proxy-panel").read_bytes(),candidate.read_bytes(),result.stdout)
                self.assertEqual((root / cli_name).read_bytes(), INSTALLER.read_bytes(), "old CLI still installed")
                usage = subprocess.check_output([str(root / cli_name), "help"], text=True)
                for command in ("share-reality", "unshare-reality", "rebind"):
                    self.assertIn("website " + command, usage)
                access = root / "units/proxy-panel.service.d/managed-website.conf"
                self.assertTrue(access.exists(), "upgraded panel needs managed Nginx write access")
                self.assertEqual(access.read_text(), f"[Service]\nReadWritePaths=-{root}/website\n")
            self.assertEqual(sorted(p.name for p in (root / "states").iterdir()),sorted(services))
            if failure in ("download","preflight","invalid-tag","missing-cli","pending-website"):
                events=(root / "events").read_text() if (root / "events").exists() else ""
                self.assertNotIn("stop ",events,result.stdout)
            return result.stdout

    def test_pending_website_recovery_blocks_update(self): self.run_case("pending-website")
    def test_pinned_update_installs_matching_cli(self): self.run_case(tag="v9.8.7")
    def test_missing_release_cli_keeps_installed_pair(self): self.run_case("missing-cli")
    def test_invalid_tag_rejected_before_download(self): self.run_case("invalid-tag", tag="../latest")
    def test_explicit_rollback_restores_original_cli(self): self.run_case(rollback=True)

    def test_download_failure_keeps_services(self): self.run_case("download")
    def test_invalid_config_keeps_services(self): self.run_case("preflight")
    def test_health_failure_restores_db_binary_config_and_services(self): self.run_case("health")
    def test_stop_failure_restores_service_state(self): self.run_case("stop")
    def test_second_preflight_failure_restores_service_state(self): self.run_case("final-preflight")
    def test_partial_replacement_restores_files(self): self.run_case("replace")
    def test_start_failure_rolls_back(self): self.run_case("start")
    def test_unit_reload_failure_restores_files_and_permissions(self): self.run_case("daemon-reload")
    def test_health_failure_restores_existing_permissions(self): self.run_case("health", existing_access=True)
    def test_success_preserves_listen(self): self.run_case()
    def test_success_without_cmp(self): self.run_case(without_cmp=True)
    def test_manifest_change_restores_services(self):
        output = self.run_case("manifest-change")
        self.assertIn("预检期间配置路径发生变化", output)
    def test_invalid_manifest_reports_comparison_error(self):
        output = self.run_case("manifest-invalid")
        self.assertIn("无法读取或比较升级预检清单", output)
        self.assertNotIn("预检期间配置路径发生变化", output)
    def test_missing_manifest_reports_comparison_error(self):
        output = self.run_case("manifest-missing")
        self.assertIn("无法读取或比较升级预检清单", output)
        self.assertNotIn("预检期间配置路径发生变化", output)
    def test_inactive_services_stay_inactive(self): self.run_case(active=False)

if __name__ == "__main__": unittest.main()
