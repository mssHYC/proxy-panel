"""Kill a real website transaction and run its installed systemd recovery entry point."""
import contextlib
import fcntl
import json
import os
from pathlib import Path
import signal
import sqlite3
import subprocess
import sys
import unittest
from unittest.mock import patch

import test_website as website


class WebsiteCrashRecoveryTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        website.WebsiteTests.setUpClass()

    @classmethod
    def tearDownClass(cls):
        website.WebsiteTests.tearDownClass()

    def setUp(self):
        self.site = site = website.WebsiteTests()
        self.addCleanup(site.doCleanups)
        site.setUp()
        site.add_independent_reality()
        with patch.object(website.tool, 'check_standalone_ports'), patch.object(website.tool, 'run', return_value=''):
            self.stage, _ = site.prepare('', 'share-reality', '2')
        self.bin = site.root / 'mocks'
        self.bin.mkdir()
        for folder in ('active', 'enabled'):
            (site.root / folder).mkdir()
        for unit in ('proxy-panel', 'xray', 'proxy-panel-web', 'proxy-panel-web-cert.path'):
            (site.root / 'active' / unit).touch()
        for unit in ('proxy-panel-web', 'proxy-panel-web-cert.path'):
            (site.root / 'enabled' / unit).touch()
        self.script(self.bin / 'systemctl', """#!/bin/bash
op=$1
shift
case "$op" in
  show) echo loaded ;;
  is-active) test -e "$ROOT/active/${!#}" ;;
  is-enabled) test -e "$ROOT/enabled/${!#}" ;;
  start|stop|enable|disable)
    for unit do
      case "$op" in
        stop) rm -f "$ROOT/active/$unit" ;;
        start)
          if [[ "$unit" == proxy-panel && -e "$ROOT/fail-restore" ]]; then exit 1; fi
          touch "$ROOT/active/$unit" ;;
        enable) touch "$ROOT/enabled/$unit" ;;
        disable) rm -f "$ROOT/enabled/$unit" ;;
      esac
    done
    if [[ "$op" == stop && "$1" == xray && "${KILL_AFTER:-}" == stop ]]; then
      "$INTERPRETER" -c 'import os,signal; os.killpg(os.getpgrp(),signal.SIGKILL)'
    fi ;;
esac
""")
        # macOS lacks the Linux flock command. Use the same inherited file descriptors.
        self.script(self.bin / 'flock', """#!/usr/bin/env python3
import fcntl, sys
args = sys.argv[1:]
flags = fcntl.LOCK_SH if '-s' in args else fcntl.LOCK_EX
if '-n' in args: flags |= fcntl.LOCK_NB
try: fcntl.flock(int(args[-1]), flags)
except BlockingIOError: sys.exit(int(args[args.index('-E') + 1]) if '-E' in args else 1)
""")
        self.script(site.root / 'proxy-panel', '#!/bin/sh\nexit 0\n')
        self.writer = site.root / 'writer.py'
        self.writer.write_text("""import os, signal, sys
from pathlib import Path
from unittest.mock import patch
from test_website import tool
replace = os.replace
def atomic_write(source, dest):
    replace(source, dest)
    if Path(dest).name == os.environ['KILL_AFTER']:
        os.killpg(os.getpgrp(), signal.SIGKILL)
with patch.object(os, 'replace', side_effect=atomic_write):
    tool.apply(Path(sys.argv[1]))
if os.environ['KILL_AFTER'] == 'after-commit':
    os.killpg(os.getpgrp(), signal.SIGKILL)
""")
        self.env = dict(os.environ, ROOT=str(site.root), INSTALLER=str(website.INSTALLER),
                        STAGED=str(self.stage), INTERPRETER=sys.executable,
                        PATH=str(self.bin) + os.pathsep + os.environ['PATH'],
                        PYTHONPATH=str(Path(website.__file__).parent))
        self.transaction = '''source "$INSTALLER"
INSTALL_DIR="$ROOT"
CONFIG_FILE="$ROOT/config.yaml"
BACKUP_DIR="$ROOT/backups"
WEBSITE_CONFIG_DIR="$ROOT/nginx"
WEBSITE_UNIT_DIR="$ROOT/units"
WEBSITE_ROOT="$ROOT/html"
setup_website_selinux() { :; }
website_tool() {
    case "$1" in
        prepare) cp -R "$STAGED" "$2" ;;
        apply) "$INTERPRETER" "$ROOT/writer.py" "$2" ;;
        health) return 0 ;;
    esac
}
run_website_setup "" share-reality 2
'''

    def script(self, path, source):
        path.write_text(source)
        path.chmod(0o755)

    def test_standalone_journal_only_blocks_website_services(self):
        setup = self.transaction.split('run_website_setup ""')[0] + 'install_website_recovery'
        subprocess.run(['bash', '-c', setup], env=self.env, check=True)
        root = self.site.root
        backup = root / 'backups/website-standalone-test'
        backup.mkdir(parents=True)
        (backup / 'state.json').write_text(json.dumps({'mode': 'standalone', 'active': [], 'enabled': [], 'files': []}))
        (root / '.website-transaction').symlink_to(backup)
        helper = str(self.site.units / 'proxy-panel-website-recovery.sh')
        self.assertEqual(subprocess.run(['bash', helper, 'gate', 'core'], env=self.env).returncode, 0)
        self.assertNotEqual(subprocess.run(['bash', helper, 'gate', 'website'], env=self.env).returncode, 0)
        subprocess.run(['bash', helper, 'recover'], env=self.env, check=True)
        self.assertTrue((root / 'active/proxy-panel').exists())
        self.assertFalse((root / '.website-transaction').is_symlink())

    def test_recover_interrupted_shared_port_before_starting_services(self):
        site = self.site
        for filename in ['stop', 'nginx.conf', 'managed.json', 'xray.json', 'after-commit']:
            with self.subTest(kill_after=filename):
                proc = subprocess.run(['bash', '-c', self.transaction], env=dict(self.env, KILL_AFTER=filename),
                                      start_new_session=True, capture_output=True, timeout=15)
                self.assertEqual(proc.returncode, -signal.SIGKILL, proc.stderr)
                journal = site.root / '.website-transaction'
                self.assertTrue(journal.is_symlink(), 'no persistent recovery record')
                helper = site.units / 'proxy-panel-website-recovery.sh'
                # Simultaneous startup probes must all deny. Only the transaction's
                # exclusive ready-to-start lock can let a guarded process proceed.
                with (site.root / '.website-start.lock').open() as other_gate:
                    fcntl.flock(other_gate, fcntl.LOCK_SH)
                    gate = subprocess.run(['bash', str(helper), 'gate'], env=self.env, capture_output=True)
                    self.assertNotEqual(gate.returncode, 0, 'mixed configurations were allowed to start')
                if filename == 'managed.json':
                    failure = site.root / 'fail-restore'
                    failure.touch()
                    retry = subprocess.run(['bash', str(helper), 'recover'], env=self.env, timeout=15)
                    self.assertNotEqual(retry.returncode, 0)
                    self.assertTrue(journal.is_symlink(), 'failed recovery lost the backup pointer')
                    failure.unlink()
                for _ in range(2):
                    subprocess.run(['bash', str(helper), 'recover'], env=self.env, check=True, timeout=15)
                self.assertFalse(journal.is_symlink())
                with contextlib.closing(sqlite3.connect(site.db)) as db:
                    self.assertEqual(db.execute('SELECT port FROM nodes WHERE id=2').fetchone(), (1279,))
                kernel = json.loads(site.kernel.read_text())
                self.assertEqual(next(i['port'] for i in kernel['inbounds'] if i['tag'] == 'node-2'), 1279)
                self.assertNotIn('reality_frontend', json.loads((site.confdir / 'managed.json').read_text()))
                self.assertNotIn('127.0.0.2:443', (site.confdir / 'nginx.conf').read_text())
                for unit in ('xray', 'proxy-panel', 'proxy-panel-web', 'proxy-panel-web-cert.path'):
                    self.assertTrue((site.root / 'active' / unit).exists(), unit)
