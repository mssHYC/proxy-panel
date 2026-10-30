"""Fault injection for the real installer flow; no root/network/services needed."""
import json
import os
from pathlib import Path
import subprocess
import tarfile
import tempfile
import unittest

INSTALLER = Path(__file__).resolve().parents[1] / "install.sh"

class UpdateTests(unittest.TestCase):
    def run_case(self, failure="", active=True):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            (root / "data").mkdir()
            (root / "states").mkdir()
            originals = {"proxy-panel": b"old-binary", "config.yaml": b"listen: 0.0.0.0", "data/panel.db": b"old-db", "xray.json": b"old-config", "xray": b"old-core"}
            for name, data in originals.items():
                (root / name).write_bytes(data)
            services = ["proxy-panel", "xray"] if active else []
            for svc in services:
                (root / "states" / svc).touch()
            candidate = root / "candidate"
            candidate.write_text('''#!/bin/bash
case "$1" in
-compatibility) echo '{"xray":"v26.10.29","sing-box":"v1.14.4"}'; exit;;
esac
for ((i=1;i<=$#;i++)); do
 if [[ "${!i}" == -check-update ]]; then
  [[ "$FAIL" != preflight ]] || exit 2
  [[ "$FAIL" != final-preflight || "${@}" != *final-check* ]] || exit 2
  j=$((i+1)); dir="${!j}"; mkdir -p "$dir"
  cp "$TEST_ROOT/manifest.json" "$dir/manifest.json"
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
            manifest = {"files": [str(root / name) for name in originals if name != "proxy-panel"] + [str(root / "data/panel.db-wal")], "xray_path":str(root / "xray"), "singbox_path":str(root / "sing-box")}
            (root / "manifest.json").write_text(json.dumps(manifest))
            script = root / "run.sh"
            script.write_text('''source "$INSTALLER"
INSTALL_DIR="$TEST_ROOT"
CONFIG_FILE="$TEST_ROOT/config.yaml"
BACKUP_DIR="$TEST_ROOT/backups"
ARCH=amd64
flock() { :; }
sleep() { :; }
confirm() { return 1; }
replace_update_binary() {
 cp "$1" "$2" && chmod 755 "$2" || return 1
 [[ "$FAIL" != replace ]]
}
fetch_update_asset() {
 [[ "$FAIL" != download ]] || return 1
 cp "$TEST_ROOT/release.tar.gz" "$4"
}
systemctl() {
 local op="$1" svc="${@: -1}"
 echo "$op $svc" >> "$TEST_ROOT/events"
 case "$op" in
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
run_safe_update
''')
            env = {**os.environ, "TEST_ROOT":temp,"INSTALLER":str(INSTALLER),"FAIL":failure}
            result = subprocess.run(["bash",str(script)],env=env,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True)
            self.assertEqual(result.returncode == 0, not failure, result.stdout)
            if failure:
                for name, data in originals.items():
                    self.assertEqual((root / name).read_bytes(), data, name + result.stdout)
                self.assertFalse((root / "data/panel.db-wal").exists(),result.stdout)
            else:
                self.assertEqual((root / "config.yaml").read_bytes(),originals["config.yaml"])
                self.assertEqual((root / "proxy-panel").read_bytes(),candidate.read_bytes(),result.stdout)
            self.assertEqual(sorted(p.name for p in (root / "states").iterdir()),sorted(services))
            if failure in ("download","preflight"):
                events=(root / "events").read_text() if (root / "events").exists() else ""
                self.assertNotIn("stop ",events,result.stdout)

    def test_download_failure_keeps_services(self): self.run_case("download")
    def test_invalid_config_keeps_services(self): self.run_case("preflight")
    def test_health_failure_restores_db_binary_config_and_services(self): self.run_case("health")
    def test_stop_failure_restores_service_state(self): self.run_case("stop")
    def test_second_preflight_failure_restores_service_state(self): self.run_case("final-preflight")
    def test_partial_replacement_restores_files(self): self.run_case("replace")
    def test_start_failure_rolls_back(self): self.run_case("start")
    def test_success_preserves_listen(self): self.run_case()
    def test_inactive_services_stay_inactive(self): self.run_case(active=False)

if __name__ == "__main__": unittest.main()
