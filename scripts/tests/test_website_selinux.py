"""Exercise the installer's SELinux check before a managed website switch."""
import errno
import json
import os
from pathlib import Path
import socket
import subprocess
import tempfile
import unittest


INSTALLER = Path(__file__).resolve().parents[1] / 'install.sh'


class SELinuxPreflightTests(unittest.TestCase):
    def run_preflight(self, backend='127.0.0.1:9443', *, enforcing=True, deny=False):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            event_log = root / 'events'
            (root / 'nginx.conf').write_text('existing nginx configuration')
            (root / 'enforce').write_text('1' if enforcing else '0')
            plan = {'mode': 'proxy', 'cert': '/tmp/staged.crt', 'key': '/tmp/staged.key'}
            if backend:
                plan['reality_frontend'] = {'web_listen': backend}
            (root / 'plan').write_text(json.dumps(plan))
            source = INSTALLER.read_text()
            source = source.replace('/sys/fs/selinux/enforce', str(root / 'enforce'))
            (root / 'installer').write_text(source)
            commands = {
                'systemd-run': 'echo "unit $*" >> "$EVENTS"\n'
                               'while [ "$1" != "$BIN/nginx" ]; do [ "$#" -gt 1 ] || exit 1; shift; done\n'
                               'TEST_FROM_UNIT=1 exec "$@"',
                'nginx': '[ "$TEST_FROM_UNIT" = 1 ] || exit 3\n'
                         'while [ "$1" != "-c" ]; do [ "$#" -gt 1 ] || exit 2; shift; done\n'
                         'cat "$2" >> "$EVENTS"\n[ "$DENY" = no ]',
                'systemctl': 'if [ "$1" = show ]; then echo "SELinuxContext=fixture_context"; fi',
                'journalctl': 'exit 0',
                'semanage': 'echo "semanage $*" >> "$EVENTS"',
                'setsebool': 'echo "setsebool $*" >> "$EVENTS"',
                'chcon': 'echo "chcon $*" >> "$EVENTS"',
                'restorecon': 'exit 0',
            }
            for command, body in commands.items():
                path = root / command
                path.write_text('#!/bin/sh\n' + body + '\n')
                path.chmod(0o755)
            env = {**os.environ, 'PATH': str(root) + os.pathsep + os.environ['PATH'], 'BIN': str(root),
                   'EVENTS': str(event_log), 'DENY': 'yes' if deny else 'no'}
            result = subprocess.run(['bash', '-c', '''
source "$BIN/installer"
WEBSITE_CONFIG_DIR="$BIN"
PKG_MGR=yum
setup_website_selinux "$BIN/plan"
'''], env=env, capture_output=True, text=True, timeout=10)
            self.assertEqual(list(root.glob('.bind-check.*')), [])
            return result, event_log.read_text() if event_log.exists() else ''

    def test_denied_service_bind_stops_preflight(self):
        result, events = self.run_preflight(deny=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('127.0.0.1:9443', result.stdout + result.stderr)
        self.assertIn('listen 127.0.0.1:9443;', events)
        self.assertNotIn('semanage port', events)

    def test_service_checks_the_planned_endpoint(self):
        result, events = self.run_preflight('127.0.0.1:8443')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('listen 127.0.0.1:8443;', events)
        self.assertIn(' -t ', events)
        self.assertIn('SELinuxContext=fixture_context', events)
        self.assertIn('setsebool -P httpd_can_network_connect on', events)

    def test_nonshared_or_permissive_plan_skips_the_service_probe(self):
        for backend, enforcing in [(None, True), ('127.0.0.1:9443', False)]:
            with self.subTest(backend=backend, enforcing=enforcing):
                result, events = self.run_preflight(backend, enforcing=enforcing, deny=True)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertNotIn('unit ', events)


    @unittest.skipUnless(os.geteuid() == 0 and os.environ.get('NGINX_BINARY')
                         and Path('/run/systemd/system').is_dir(), 'requires root, systemd and Nginx')
    def test_systemd_propagates_actual_nginx_bind_result(self):
        # Ubuntu CI has no SELinux labels. Exercise the real systemd unit and Nginx socket check.
        unavailable = '192.0.2.254'
        with socket.socket() as sock:
            try:
                sock.bind((unavailable, 0))
            except OSError as exc:
                self.assertEqual(exc.errno, errno.EADDRNOTAVAIL)
            else:
                self.skipTest('host allows binding the reserved test address')
        with socket.socket() as sock:
            sock.bind(('127.0.0.1', 0))
            port = sock.getsockname()[1]
        with tempfile.TemporaryDirectory() as tmp:
            (Path(tmp) / 'nginx.conf').touch()
            env = {**os.environ, 'INSTALLER': str(INSTALLER), 'PROBE_DIR': tmp,
                   'PATH': str(Path(os.environ['NGINX_BINARY']).parent) + os.pathsep + os.environ['PATH']}
            for address, success in [('127.0.0.1', True), (unavailable, False)]:
                with self.subTest(address=address):
                    result = subprocess.run(['bash', '-c', '''
source "$INSTALLER"
WEBSITE_CONFIG_DIR="$PROBE_DIR"
WEBSITE_SERVICE=systemd-journald.service
chcon() { :; }
check_website_selinux_bind "$1"
''', 'probe', f'{address}:{port}'], env=env, text=True, capture_output=True, timeout=15)
                    self.assertEqual(result.returncode == 0, success, result.stdout + result.stderr)


if __name__ == '__main__':
    unittest.main()
