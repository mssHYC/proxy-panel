"""Run the installer's crash watcher and startup gates under real systemd."""
import os
from pathlib import Path
import shutil
import signal
import subprocess
import tempfile
import time
import unittest


INSTALLER = Path(__file__).resolve().parents[1] / 'install.sh'


@unittest.skipUnless(os.environ.get('SYSTEMD_ANALYZE_BINARY'), 'requires a root systemd runner')
class WebsiteSystemdRecoveryTests(unittest.TestCase):
    def test_active_watcher_recovers_killed_deployment(self):
        self.check_recovery(restart_watcher=False)

    def test_starting_watcher_recovers_journal_left_on_disk(self):
        self.check_recovery(restart_watcher=True)

    def check_recovery(self, restart_watcher):
        units = Path('/run/systemd/system')
        self.assertEqual(os.geteuid(), 0)
        with tempfile.TemporaryDirectory(dir='/run', prefix='pp-recovery-test-') as tmp:
            root = Path(tmp)
            prefix = root.name
            web = prefix + '-web.service'
            cert = prefix + '-web-cert.path'
            watcher = prefix + '-website-recover.path'
            worker = prefix + '-website-recover.service'
            self.assertFalse(list(units.glob(prefix + '*')))

            def ctl(*args, check=True):
                return subprocess.run(['systemctl', *args], check=check, text=True,
                                      capture_output=True, timeout=20)

            # Only unit names and paths are redirected. PID 1 loads the generated
            # recovery worker and drop-ins and runs their original ExecStart commands.
            installer = root / 'install.sh'
            installer.write_text(INSTALLER.read_text().replace('proxy-panel', prefix))
            (root / 'managed.json').write_text('committed\n')
            history = root / 'served'
            (units / web).write_text(f"""[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=/bin/sh -c 'cat {root}/managed.json >> {history}'
[Install]
WantedBy=multi-user.target
""")
            (units / cert).write_text(f"""[Path]
PathExists={root}/unused-cert-event
Unit={web}
[Install]
WantedBy=multi-user.target
""")
            # The stub supplies a single managed file and pauses inside apply.
            # The CLI still captures real unit state, publishes the backup,
            # stops the website, and registers its own recovery units.
            deploy = r'''
source "$1/install.sh"
INSTALL_DIR=$1 BACKUP_DIR=$1/backups WEBSITE_CONFIG_DIR=$1 WEBSITE_UNIT_DIR=/run/systemd/system
SERVICE_NAME=$2-panel XRAY_SERVICE=$2-xray SINGBOX_SERVICE=$2-sing-box WEBSITE_SERVICE=$2-web
setup_website_selinux() { :; }
website_tool() {
    case "$1" in
        prepare)
            mkdir -p "$2"
            jq -n --arg f "$INSTALL_DIR/managed.json" '{mode:"standalone",files:[$f]}' > "$2/plan.json" ;;
        apply)
            echo partial > "$INSTALL_DIR/managed.json"
            touch "$INSTALL_DIR/applying"
            sleep 120 ;;
    esac
}
run_website_setup
'''
            process = None
            try:
                ctl('daemon-reload')
                ctl('start', web)
                with (root / 'deployment.log').open('w') as log:
                    process = subprocess.Popen(['bash', '-c', deploy, 'deploy-test', tmp, prefix],
                                               stdout=log, stderr=subprocess.STDOUT, start_new_session=True)
                deadline = time.monotonic() + 25
                while not (root / 'applying').exists():
                    if process.poll() is not None or time.monotonic() > deadline:
                        self.fail((root / 'deployment.log').read_text())
                    time.sleep(.05)
                if restart_watcher:
                    # At boot the path unit starts with a journal already on disk.
                    ctl('stop', watcher)
                    ctl('stop', worker)
                # A start requested while the transaction owns mixed files must fail
                # before the service's ExecStart sees them.
                self.assertNotEqual(ctl('start', web, check=False).returncode, 0)
                self.assertEqual(history.read_text().splitlines(), ['committed'])
                os.killpg(process.pid, signal.SIGKILL)
                self.assertEqual(process.wait(timeout=5), -signal.SIGKILL)

                pending = root / '.website-transaction'
                if restart_watcher:
                    self.assertTrue(pending.is_symlink())
                    ctl('start', watcher)
                deadline = time.monotonic() + 20
                while pending.is_symlink() or ctl('is-active', '--quiet', web, check=False).returncode:
                    if time.monotonic() > deadline:
                        detail = subprocess.run(['journalctl', '-b', '--no-pager', '-u', worker, '-u', web],
                                                capture_output=True, text=True, timeout=10)
                        self.fail(detail.stdout)
                    time.sleep(.1)
                self.assertEqual(history.read_text().splitlines(), ['committed', 'committed'])
                self.assertEqual((root / 'managed.json').read_text(), 'committed\n')
            finally:
                if process is not None and process.poll() is None:
                    os.killpg(process.pid, signal.SIGKILL)
                    process.wait(timeout=5)
                ctl('stop', watcher, check=False)
                ctl('stop', worker, web, cert, check=False)
                ctl('disable', watcher, web, cert, check=False)
                for entry in units.glob(prefix + '*'):
                    if entry.is_dir():
                        shutil.rmtree(entry)
                    else:
                        entry.unlink()
                ctl('daemon-reload')
                ctl('reset-failed', worker, web, check=False)


if __name__ == '__main__':
    unittest.main()
