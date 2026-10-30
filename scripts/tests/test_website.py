"""Exercise the website helper shipped inside the standalone management script."""
import contextlib
import copy
import json
import os
from pathlib import Path
import socket
import sqlite3
import ssl
import subprocess
import tempfile
import threading
import time
import types
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from unittest.mock import patch

INSTALLER = Path(__file__).resolve().parents[1] / 'install.sh'
SOURCE = INSTALLER.read_text().split("<<'PY_WEBSITE'\n", 1)[1].split('\nPY_WEBSITE', 1)[0]
tool = types.ModuleType('website_tool')
exec(compile(SOURCE, str(INSTALLER), 'exec'), tool.__dict__)


class WebsiteTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.cert_temp = tempfile.TemporaryDirectory()
        cls.cert = Path(cls.cert_temp.name) / 'site.crt'
        cls.key = Path(cls.cert_temp.name) / 'site.key'
        subprocess.run(['openssl', 'req', '-x509', '-newkey', 'rsa:2048', '-nodes', '-days', '2',
                        '-subj', '/CN=site.example.com', '-addext', 'subjectAltName=DNS:site.example.com,DNS:cdn.example.com',
                        '-keyout', str(cls.key), '-out', str(cls.cert)], check=True, capture_output=True)

    @classmethod
    def tearDownClass(cls):
        cls.cert_temp.cleanup()

    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.db = self.root / 'panel.db'
        self.settings = {'security': 'tls', 'path': '/csda', 'host': 'cdn.example.com',
                         'sni': 'cdn.example.com', 'listen': '0.0.0.0', 'alpn': ['h2', 'http/1.1'],
                         'cert_path': str(self.cert), 'key_path': str(self.key), 'fingerprint': 'chrome'}
        with contextlib.closing(sqlite3.connect(self.db)) as db, db:
            db.execute('CREATE TABLE nodes (id INTEGER,host TEXT,protocol TEXT,transport TEXT,settings TEXT,enable INTEGER,port INTEGER,kernel_type TEXT,updated_at TEXT)')
            db.execute('INSERT INTO nodes VALUES (1,?,?,?,?,1,443,"xray",NULL)',
                       ('cdn.example.com', 'vless', 'httpupgrade', json.dumps(self.settings)))
        self.kernel = self.root / 'xray.json'
        self.original_core = {'inbounds': [{'tag': 'node-1', 'listen': '0.0.0.0', 'port': 443, 'protocol': 'vless',
            'settings': {'decryption': 'none', 'clients': [{'id': '98a3e56d-df22-43d7-b12a-714f2c863e25'}]},
            'streamSettings': {'network': 'httpupgrade', 'security': 'tls',
                'httpupgradeSettings': {'path': '/csda', 'host': 'cdn.example.com'},
                'tlsSettings': {'serverName': 'cdn.example.com', 'rejectUnknownSni': True, 'alpn': ['h2', 'http/1.1'],
                    'certificates': [{'certificateFile': str(self.cert), 'keyFile': str(self.key)}]}}}],
            'outbounds': [{'protocol': 'freedom'}]}
        self.kernel.write_text(json.dumps(self.original_core))
        self.config = self.root / 'config.yaml'
        self.config.write_text(json.dumps({'server': {'domain': 'site.example.com', 'listen': '0.0.0.0', 'port': 12597},
            'database': {'path': str(self.db)}, 'kernel': {'xray_config': str(self.kernel), 'xray_path': os.environ.get('XRAY_BINARY', '/fixture/xray')}}))
        self.confdir, self.webroot, self.units = [self.root / p for p in ('nginx', 'html', 'units')]
        self.nginx = os.environ.get('NGINX_BINARY', '/fixture/nginx')
        self.real_run = tool.run
        self.stack = contextlib.ExitStack()
        self.addCleanup(self.stack.close)
        self.stack.enter_context(patch.object(tool, 'interface_ips', return_value=['192.0.2.10', '2001:db8::10']))
        self.stack.enter_context(patch.object(tool.shutil, 'which', return_value=self.nginx))
        self.stack.enter_context(patch.object(tool.ssl, 'get_default_verify_paths', return_value=types.SimpleNamespace(cafile=str(self.cert))))
        self.stack.enter_context(patch.object(tool, 'run', side_effect=self.checked_run))

    def checked_run(self, args):
        if args[0].startswith('/fixture/'):
            return ''
        if args[0] == 'systemd-analyze':
            if not os.environ.get('SYSTEMD_ANALYZE_BINARY'):
                return ''
            args = [os.environ['SYSTEMD_ANALYZE_BINARY'], *args[1:]]
        return self.real_run(args)

    def prepare(self, vmess_ids=''):
        stage = self.root / 'stage'
        tool.prepare(str(self.config), str(self.root), self.confdir, self.webroot, self.units, stage, vmess_ids=vmess_ids)
        return stage, json.loads((stage / 'plan.json').read_text())

    def add_vmess(self, transport='ws', path='/vmess', port=18080):
        settings = dict(self.settings, path=path)
        with contextlib.closing(sqlite3.connect(self.db)) as db, db:
            db.execute('INSERT INTO nodes VALUES (2,?,?,?,?,1,?,"xray",NULL)',
                       ('cdn.example.com', 'vmess', transport, json.dumps(settings), port))
        core = json.loads(self.kernel.read_text())
        inbound = copy.deepcopy(core['inbounds'][0])
        inbound.update(tag='node-2', port=port, protocol='vmess')
        inbound['settings'].pop('decryption')
        stream = inbound['streamSettings']
        stream['network'] = transport
        stream.pop('httpupgradeSettings')
        if transport == 'ws':
            stream['wsSettings'] = {'path': path, 'headers': {'Host': settings['host']}}
        elif transport == 'httpupgrade':
            stream['httpupgradeSettings'] = {'path': path, 'host': settings['host']}
        core['inbounds'].append(inbound)
        self.kernel.write_text(json.dumps(core))
        return settings, inbound

    def test_selected_vmess_joins_443_without_changing_backend_port_or_credentials(self):
        original, inbound = self.add_vmess()
        stage, plan = self.prepare('2')
        self.assertEqual(self.node_settings(), self.settings)
        self.assertEqual([r['port'] for r in plan['routes']], [443, 18080])
        tool.apply(stage)
        with contextlib.closing(sqlite3.connect(self.db)) as db:
            port, settings = db.execute('SELECT port,settings FROM nodes WHERE id=2').fetchone()
        self.assertEqual(port, 18080)
        self.assertEqual(json.loads(settings), dict(original, listen='127.0.0.1', public_port=443, alpn=['http/1.1']))
        core = json.loads(self.kernel.read_text())
        self.assertEqual(core['inbounds'][1]['settings'], inbound['settings'])
        self.assertEqual(core['inbounds'][1]['listen'], '127.0.0.1')
        conf = (self.confdir / 'nginx.conf').read_text()
        self.assertIn('location = "/csda"', conf)
        self.assertIn('location = "/vmess"', conf)
        self.assertIn('proxy_pass https://127.0.0.1:18080;', conf)
        self.assertNotIn('listen 192.0.2.10:18080', conf)
        state = json.loads((self.confdir / 'managed.json').read_text())
        self.assertEqual(state['vmess_ids'], [2])
        self.assertNotIn('original_settings', json.dumps(state))
        self.assertNotIn('changes', state)
        stage, repeated = self.prepare()
        self.assertEqual(repeated['vmess_ids'], [2])
        self.assertEqual(len(repeated['routes']), 2)

    def test_unselected_vmess_is_untouched(self):
        original, _ = self.add_vmess(transport='tcp')
        stage, plan = self.prepare()
        tool.apply(stage)
        with contextlib.closing(sqlite3.connect(self.db)) as db:
            self.assertEqual(json.loads(db.execute('SELECT settings FROM nodes WHERE id=2').fetchone()[0]), original)
        self.assertEqual(len(plan['routes']), 1)

    def test_vmess_tcp_and_duplicate_paths_fail_before_changes(self):
        self.add_vmess(transport='tcp')
        with self.assertRaisesRegex(ValueError, 'WS/HTTPUpgrade'):
            self.prepare('2')
        with contextlib.closing(sqlite3.connect(self.db)) as db, db:
            db.execute('DELETE FROM nodes WHERE id=2')
        self.kernel.write_text(json.dumps(self.original_core))
        self.add_vmess(path='/csda')
        with self.assertRaisesRegex(ValueError, '路径重复'):
            self.prepare('2')
        self.assertEqual(self.node_settings(), self.settings)
        self.assertFalse(self.confdir.exists())

    def test_second_node_race_rolls_back_all_database_changes(self):
        self.add_vmess()
        stage, _ = self.prepare('2')
        with contextlib.closing(sqlite3.connect(self.db)) as db, db:
            db.execute('UPDATE nodes SET host="changed.example.com" WHERE id=2')
        with self.assertRaisesRegex(ValueError, '节点在预检后发生变化'):
            tool.apply(stage)
        self.assertEqual(self.node_settings(), self.settings)
        self.assertFalse(self.confdir.exists())

    def test_disabled_vmess_route_is_removed_on_next_setup(self):
        self.add_vmess()
        stage, _ = self.prepare('2')
        tool.apply(stage)
        with contextlib.closing(sqlite3.connect(self.db)) as db, db:
            db.execute('UPDATE nodes SET enable=0 WHERE id=2')
        core = json.loads(self.kernel.read_text())
        core['inbounds'] = core['inbounds'][:1]
        self.kernel.write_text(json.dumps(core))
        stage, plan = self.prepare()
        self.assertEqual(len(plan['routes']), 1)
        self.assertEqual(plan['vmess_ids'], [2])
        tool.apply(stage)
        self.assertNotIn('/vmess', (self.confdir / 'nginx.conf').read_text())

    def test_invalid_or_missing_vmess_id_is_rejected(self):
        for ids in ('0', '2,', 'abc', '2'):
            with self.subTest(ids=ids), self.assertRaises(ValueError):
                self.prepare(ids)
        self.assertEqual(self.node_settings(), self.settings)
        self.assertFalse(self.confdir.exists())

    def node_settings(self):
        with contextlib.closing(sqlite3.connect(self.db)) as db, db:
            return json.loads(db.execute('SELECT settings FROM nodes WHERE id=1').fetchone()[0])

    def test_prepare_and_apply_preserve_subscription_and_credentials(self):
        before = self.kernel.read_bytes()
        stage, plan = self.prepare()
        self.assertEqual(self.node_settings(), self.settings)
        self.assertEqual(self.kernel.read_bytes(), before)
        self.assertFalse(self.confdir.exists())
        self.assertEqual(plan['domains'], ['cdn.example.com', 'site.example.com'])
        self.assertEqual(plan['listens'], ['192.0.2.10:443'])
        tool.apply(stage)
        wanted = dict(self.settings, listen='127.0.0.1', alpn=['http/1.1'])
        self.assertEqual(self.node_settings(), wanted)
        updated = json.loads(self.kernel.read_text())['inbounds'][0]
        self.assertEqual(updated['settings'], self.original_core['inbounds'][0]['settings'])
        self.assertEqual(updated['port'], 443)
        conf = (self.confdir / 'nginx.conf').read_text()
        self.assertIn('listen 192.0.2.10:443 ssl', conf)
        self.assertIn('proxy_pass https://127.0.0.1:443', conf)
        self.assertIn('location = "/csda"', conf)
        self.assertIn('proxy_ssl_verify on', conf)
        self.assertIn('ssl_reject_handshake on', conf)
        self.assertNotIn('12597', conf)
        self.assertIn('PathChanged=', (self.units / 'proxy-panel-web-cert.path').read_text())
        self.assertEqual(self.webroot.stat().st_mode & 0o777, 0o755)

    def test_repeated_setup_preserves_custom_homepage(self):
        stage, _ = self.prepare()
        tool.apply(stage)
        (self.webroot / 'index.html').write_text('my existing website')
        stage, _ = self.prepare()
        tool.apply(stage)
        self.assertEqual((self.webroot / 'index.html').read_text(), 'my existing website')

    def test_retry_accepts_empty_directories_left_by_rollback(self):
        self.confdir.mkdir()
        self.webroot.mkdir()
        stage, _ = self.prepare()
        tool.apply(stage)
        self.assertTrue((self.confdir / 'managed.json').is_file())

    def test_certificate_watch_paths_are_literal_absolute_paths(self):
        units = tool.website_units('/usr/sbin/nginx', '/etc/nginx/proxy-panel/nginx.conf',
                                   '/opt/cert folder/100%.crt', '/opt/cert folder/100%.key')
        paths = [line for line in units['proxy-panel-web-cert.path'].splitlines() if line.startswith('PathChanged=')]
        self.assertEqual(paths, ['PathChanged=/opt/cert folder/100%%.crt',
                                 'PathChanged=/opt/cert folder/100%%.key'])

    def test_invalid_systemd_unit_fails_before_node_changes(self):
        def reject_unit(args):
            if args[0] == 'systemd-analyze':
                raise ValueError('bad unit file setting')
            return self.checked_run(args)
        with patch.object(tool, 'run', side_effect=reject_unit):
            with self.assertRaisesRegex(ValueError, 'bad unit file setting'):
                self.prepare()
        self.assertEqual(self.node_settings(), self.settings)
        self.assertEqual(json.loads(self.kernel.read_text()), self.original_core)
        self.assertFalse(self.confdir.exists())

    @unittest.skipUnless(os.environ.get('SYSTEMD_ANALYZE_BINARY'), 'set SYSTEMD_ANALYZE_BINARY for Linux unit verification')
    def test_real_systemd_rejects_original_quoted_watch_paths(self):
        stage, _ = self.prepare()
        watcher = stage / 'proxy-panel-web-cert.path'
        text = watcher.read_text()
        for cert_path in (self.cert, self.key):
            text = text.replace('PathChanged=' + str(cert_path), 'PathChanged="' + str(cert_path) + '"')
        watcher.write_text(text)
        with self.assertRaises(ValueError):
            self.real_run([os.environ['SYSTEMD_ANALYZE_BINARY'], 'verify', '--man=no', str(watcher)])

    @unittest.skipUnless(os.environ.get('SYSTEMD_ANALYZE_BINARY'), 'set SYSTEMD_ANALYZE_BINARY on a systemd runner')
    def test_real_systemd_watcher_starts_and_detects_both_renewals(self):
        # Use an isolated unit name and a harmless reload handler on the CI runner.
        name = 'proxy-panel-website-test-' + str(os.getpid())
        cert, key, marker = [self.root / p for p in ('watched cert%.crt', 'watched key%.key', 'reloaded')]
        cert.touch()
        key.touch()
        contents = tool.website_units(self.nginx, self.confdir / 'nginx.conf', cert, key)
        watcher, handler = self.root / (name + '.path'), self.root / (name + '.service')
        watcher.write_text(contents['proxy-panel-web-cert.path'].replace('Unit=proxy-panel-web-reload.service', 'Unit=' + handler.name))
        handler.write_text('[Service]\nType=oneshot\nExecStart=/bin/touch ' + str(marker) + '\n')

        def cleanup():
            subprocess.run(['systemctl', 'stop', watcher.name, handler.name], capture_output=True)
            for unit in (watcher, handler):
                link = Path('/etc/systemd/system') / unit.name
                if link.is_symlink() and link.resolve() == unit:
                    link.unlink()
            subprocess.run(['systemctl', 'daemon-reload'], capture_output=True)
        self.addCleanup(cleanup)
        self.real_run(['systemctl', 'link', str(watcher), str(handler)])
        self.real_run(['systemctl', 'daemon-reload'])
        self.real_run(['systemctl', 'start', watcher.name])
        self.real_run(['systemctl', 'is-active', '--quiet', watcher.name])
        for changed in (cert, key):
            marker.unlink(missing_ok=True)
            changed.write_text('renewed')
            reloaded = False
            for _ in range(50):
                if marker.exists() and self.real_run(['systemctl', 'show', '-p', 'ActiveState', '--value', handler.name]).strip() == 'inactive':
                    reloaded = True
                    break
                time.sleep(.1)
            self.assertTrue(reloaded, 'Certificate watcher did not trigger: ' + str(changed))

    def test_wrong_cdn_certificate_fails_without_mutation(self):
        with contextlib.closing(sqlite3.connect(self.db)) as db, db:
            wrong = dict(self.settings, sni='wrong.example.com')
            db.execute('UPDATE nodes SET settings=?', (json.dumps(wrong),))
        with self.assertRaisesRegex(ValueError, '证书.*wrong.example.com'):
            self.prepare()
        self.assertEqual(self.node_settings(), wrong)
        self.assertFalse(self.confdir.exists())

    def test_certificate_symlinks_are_preserved_for_renewal(self):
        cert_link, key_link = self.root / 'live.crt', self.root / 'live.key'
        cert_link.symlink_to(self.cert)
        key_link.symlink_to(self.key)
        settings = dict(self.settings, cert_path=str(cert_link), key_path=str(key_link))
        with contextlib.closing(sqlite3.connect(self.db)) as db, db:
            db.execute('UPDATE nodes SET settings=?', (json.dumps(settings),))
        live = json.loads(self.kernel.read_text())
        live['inbounds'][0]['streamSettings']['tlsSettings']['certificates'][0] = {
            'certificateFile': str(cert_link), 'keyFile': str(key_link)}
        self.kernel.write_text(json.dumps(live))
        stage, plan = self.prepare()
        self.assertEqual(plan['cert'], str(cert_link))
        self.assertIn(str(cert_link), (stage / 'proxy-panel-web-cert.path').read_text())

    def test_other_transport_is_not_changed(self):
        with contextlib.closing(sqlite3.connect(self.db)) as db, db:
            db.execute('UPDATE nodes SET transport="tcp"')
        with self.assertRaisesRegex(ValueError, 'HTTPUpgrade'):
            self.prepare()
        self.assertEqual(self.node_settings(), self.settings)

    def test_existing_unmanaged_site_is_not_overwritten(self):
        self.webroot.mkdir()
        (self.webroot / 'index.html').write_text('existing')
        with self.assertRaisesRegex(ValueError, '拒绝覆盖'):
            self.prepare()
        self.assertEqual((self.webroot / 'index.html').read_text(), 'existing')

    def test_concurrent_node_edit_is_not_overwritten(self):
        stage, _ = self.prepare()
        with contextlib.closing(sqlite3.connect(self.db)) as db, db:
            db.execute('UPDATE nodes SET host="new.example.com"')
        with self.assertRaisesRegex(ValueError, '节点在预检后发生变化'):
            tool.apply(stage)
        self.assertFalse(self.confdir.exists())

    def test_stale_core_config_is_rejected(self):
        live = json.loads(self.kernel.read_text())
        live['inbounds'][0]['streamSettings']['httpupgradeSettings']['path'] = '/other'
        self.kernel.write_text(json.dumps(live))
        with self.assertRaisesRegex(ValueError, '请先同步内核'):
            self.prepare()

    def test_wildcard_listens_exclude_loopback_and_preserve_family(self):
        self.assertEqual(tool.select_listens('0.0.0.0', ['192.0.2.10', '2001:db8::10']), ['192.0.2.10:443'])
        self.assertEqual(tool.select_listens('::', ['192.0.2.10', '2001:db8::10']), ['192.0.2.10:443', '[2001:db8::10]:443'])
        with self.assertRaisesRegex(ValueError, '不会接管'):
            tool.select_listens('127.0.0.1', ['192.0.2.10'])

    @unittest.skipUnless(os.environ.get('NGINX_BINARY'), 'set NGINX_BINARY for real HTTPS/Upgrade integration')
    def test_real_nginx_serves_site_and_tunnels_httpupgrade(self):
        stage, plan = self.prepare()
        self.root.chmod(0o755)
        self.webroot.mkdir()
        (self.webroot / 'index.html').write_text('website-ok')
        observed = []

        class Backend(BaseHTTPRequestHandler):
            protocol_version = 'HTTP/1.1'
            def log_message(self, *args): pass
            def do_GET(self):
                observed.append((self.path, self.headers.get('Host'), self.headers.get('Upgrade')))
                self.send_response(101)
                self.send_header('Upgrade', 'websocket')
                self.send_header('Connection', 'Upgrade')
                self.end_headers()
                data = self.rfile.read(4)
                self.wfile.write(data.upper())
                self.wfile.flush()
                self.close_connection = True

        backend = ThreadingHTTPServer(('127.0.0.1', 0), Backend)
        tls = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
        tls.load_cert_chain(self.cert, self.key)
        backend.socket = tls.wrap_socket(backend.socket, server_side=True)
        threading.Thread(target=backend.serve_forever, daemon=True).start()
        self.addCleanup(backend.server_close)
        self.addCleanup(backend.shutdown)
        with socket.socket() as probe:
            probe.bind(('127.0.0.1', 0))
            port = probe.getsockname()[1]
        plan['listens'] = [f'127.0.0.1:{port}']
        conf = tool.nginx_config(plan, self.webroot)
        conf = conf.replace('pid /run/proxy-panel-web.pid;', f'pid {self.root}/nginx.pid;')
        conf = conf.replace('https://127.0.0.1:443;', f'https://127.0.0.1:{backend.server_port};')
        test_conf = self.root / 'integration.conf'
        test_conf.write_text(conf)
        # CI runs the master as root, as deployment does; its workers must still read the site.
        log = (self.root / 'nginx.log').open('w+')
        self.addCleanup(log.close)
        process = subprocess.Popen([self.nginx, '-p', str(self.root), '-c', str(test_conf), '-g', 'daemon off;'], stdout=log, stderr=log)
        self.addCleanup(lambda: (process.terminate(), process.wait(timeout=5)) if process.poll() is None else None)
        for _ in range(40):
            if process.poll() is not None:
                log.seek(0)
                self.fail(log.read())
            try:
                with socket.create_connection(('127.0.0.1', port), timeout=.1): break
            except OSError: time.sleep(.05)
        ctx = ssl.create_default_context(cafile=str(self.cert))
        for name in plan['domains']:
            with socket.create_connection(('127.0.0.1', port), timeout=3) as sock:
                with ctx.wrap_socket(sock, server_hostname=name) as conn:
                    conn.sendall(f'GET / HTTP/1.1\r\nHost: {name}\r\nConnection: close\r\n\r\n'.encode())
                    response = conn.makefile('rb').read()
                    self.assertIn(b'200 OK', response)
                    self.assertIn(b'website-ok', response)
        with socket.create_connection(('127.0.0.1', port), timeout=3) as sock:
            with ctx.wrap_socket(sock, server_hostname='cdn.example.com') as conn:
                conn.sendall(b'GET /csda HTTP/1.1\r\nHost: cdn.example.com\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\n')
                stream = conn.makefile('rb')
                self.assertIn(b'101', stream.readline())
                while stream.readline() != b'\r\n': pass
                conn.sendall(b'ping')
                self.assertEqual(stream.read(4), b'PING')
        self.assertEqual(observed, [('/csda', 'cdn.example.com', 'websocket')])
        with socket.create_connection(('127.0.0.1', port), timeout=3) as sock:
            with self.assertRaises(ssl.SSLError):
                ctx.wrap_socket(sock, server_hostname='wrong.example.com')

    @unittest.skipUnless(os.environ.get('NGINX_BINARY') and os.environ.get('XRAY_BINARY'), 'requires Nginx and Xray')
    def test_real_vmess_ws_data_through_shared_nginx(self):
        self.check_real_vmess('ws')

    @unittest.skipUnless(os.environ.get('NGINX_BINARY') and os.environ.get('XRAY_BINARY'), 'requires Nginx and Xray')
    def test_real_vmess_httpupgrade_data_through_shared_nginx(self):
        self.check_real_vmess('httpupgrade')

    def check_real_vmess(self, transport):
        self.add_vmess(transport=transport)
        stage, plan = self.prepare('2')
        self.root.chmod(0o755)
        self.webroot.mkdir()
        (self.webroot / 'index.html').write_text('website-ok')

        def unused_port():
            with socket.socket() as sock:
                sock.bind(('127.0.0.1', 0))
                return sock.getsockname()[1]

        def start(args, port, name):
            log = (self.root / (name + '.log')).open('w+')
            self.addCleanup(log.close)
            process = subprocess.Popen(args, stdout=log, stderr=log)
            self.addCleanup(lambda: (process.terminate(), process.wait(timeout=5)) if process.poll() is None else None)
            for _ in range(100):
                if process.poll() is not None:
                    log.seek(0)
                    self.fail(log.read())
                try:
                    with socket.create_connection(('127.0.0.1', port), timeout=.1): return
                except OSError: time.sleep(.05)
            self.fail(name + ' did not listen')

        # Isolated high ports: no production listeners or system services are changed.
        core = json.loads((stage / 'xray.json').read_text())
        core['log'] = {'loglevel': 'debug'}
        for inbound, route in zip(core['inbounds'], plan['routes']):
            inbound['port'] = route['port'] = unused_port()
        server_config = self.root / 'server.json'
        server_config.write_text(json.dumps(core))
        xray = os.environ['XRAY_BINARY']
        start([xray, 'run', '-config', str(server_config)], plan['routes'][1]['port'], 'server')
        public_port = unused_port()
        plan['listens'] = [f'127.0.0.1:{public_port}']
        conf = tool.nginx_config(plan, self.webroot).replace('pid /run/proxy-panel-web.pid;', f'pid {self.root}/nginx.pid;')
        nginx_conf = self.root / 'nginx-test.conf'
        nginx_conf.write_text(conf)
        start([self.nginx, '-p', str(self.root), '-c', str(nginx_conf), '-g', 'daemon off;'], public_port, 'nginx')

        ctx = ssl.create_default_context(cafile=str(self.cert))
        with socket.create_connection(('127.0.0.1', public_port), timeout=3) as sock:
            with ctx.wrap_socket(sock, server_hostname='cdn.example.com') as conn:
                conn.sendall(b'GET / HTTP/1.1\r\nHost: cdn.example.com\r\nConnection: close\r\n\r\n')
                self.assertIn(b'website-ok', conn.makefile('rb').read())

        proxy_port = unused_port()
        stream = copy.deepcopy(core['inbounds'][1]['streamSettings'])
        stream['tlsSettings'] = {'serverName': 'cdn.example.com', 'alpn': ['http/1.1'],
            'disableSystemRoot': True, 'certificates': [{'certificateFile': str(self.cert), 'usage': 'verify'}]}
        client = {'log': {'loglevel': 'debug'}, 'inbounds': [{'listen': '127.0.0.1', 'port': proxy_port, 'protocol': 'http'}],
            'outbounds': [{'protocol': 'vmess', 'settings': {'vnext': [{'address': '127.0.0.1', 'port': public_port,
                'users': [{'id': core['inbounds'][1]['settings']['clients'][0]['id'], 'security': 'auto'}]}]},
                'streamSettings': stream}]}
        client_config = self.root / 'client.json'
        client_config.write_text(json.dumps(client))
        start([xray, 'run', '-config', str(client_config)], proxy_port, 'client')
        # The pinned Xray freedom outbound rejects local targets, even without panel routing rules.
        # Use the same external positive control as the core integration suite.
        response = subprocess.run(['curl', '-fsS', '--max-time', '12', '--noproxy', '',
            '--proxy', f'http://127.0.0.1:{proxy_port}', 'https://1.1.1.1/cdn-cgi/trace'], capture_output=True, text=True)
        self.assertEqual(response.returncode, 0, response.stderr + (self.root / 'client.log').read_text() + (self.root / 'server.log').read_text() + (self.root / 'nginx.log').read_text())
        self.assertIn('ip=', response.stdout)


class WebsiteDependencyTests(unittest.TestCase):
    def run_deps(self, manager, missing_nginx=False, missing_yaml=False, fail=False):
        with tempfile.TemporaryDirectory() as temp:
            script = Path(temp) / 'deps.sh'
            events = Path(temp) / 'events'
            script.write_text('''source "$INSTALLER"
PKG_MGR="$MANAGER"
command() {
 if [[ "$1" == -v ]]; then [[ "$2" != nginx || "$NGINX_MISSING" != 1 ]]; else builtin command "$@"; fi
}
python3() { [[ "$2" != 'import yaml' || "$YAML_MISSING" != 1 ]]; }
yum() {
 echo "yum $*" >> "$EVENTS"
 [[ "$FAIL" != 1 ]] || return 2
 NGINX_MISSING=0; YAML_MISSING=0
}
apt-get() {
 echo "apt-get $*" >> "$EVENTS"
 [[ "$FAIL" != 1 ]] || return 2
 YAML_MISSING=0
}
systemctl() { echo "systemctl $*" >> "$EVENTS"; }
install_website_deps
''')
            env = {**os.environ, 'INSTALLER': str(INSTALLER), 'MANAGER': manager, 'EVENTS': str(events),
                   'NGINX_MISSING': str(int(missing_nginx)), 'YAML_MISSING': str(int(missing_yaml)), 'FAIL': str(int(fail))}
            result = subprocess.run(['bash', str(script)], env=env, capture_output=True, text=True, timeout=10)
            self.assertEqual(result.returncode == 0, not fail, result.stderr)
            return events.read_text() if events.exists() else ''

    def test_almalinux_installs_nginx_and_python_yaml(self):
        events = self.run_deps('yum', missing_nginx=True, missing_yaml=True)
        self.assertIn('yum install -y nginx python3-pyyaml', events)
        self.assertIn('systemctl disable --now nginx', events)

    def test_existing_nginx_is_not_upgraded_or_restarted(self):
        for manager in ('apt', 'yum'):
            events = self.run_deps(manager, missing_yaml=True)
            self.assertNotIn('nginx', events)
            self.assertNotIn('systemctl', events)

    def test_dependencies_already_present_make_no_package_changes(self):
        self.assertEqual(self.run_deps('apt'), '')

    def test_package_install_failure_does_not_stop_services(self):
        events = self.run_deps('yum', missing_nginx=True, fail=True)
        self.assertNotIn('systemctl', events)


class WebsiteTransactionTests(unittest.TestCase):
    def run_case(self, failure='', offer=False, vmess=False):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            (root / 'states').mkdir()
            (root / 'enabled').mkdir()
            for svc in ['proxy-panel', 'xray']:
                (root / 'states' / svc).touch()
            originals = {'panel.db': b'old-db', 'xray.json': b'old-core', 'config.yaml': b'unchanged-listen'}
            new_files = ['site/nginx.conf', 'site/managed.json', 'html/index.html', 'units/proxy-panel-web.service',
                         'units/proxy-panel-web-reload.service', 'units/proxy-panel-web-cert.path']
            for name, value in originals.items():
                (root / name).write_bytes(value)
            panel = root / 'proxy-panel'
            panel.write_text('#!/bin/bash\nif [[ "$1" == -compatibility && "$FAIL" != capability ]]; then echo \'{"features":["vmess-public-port"]}\'; fi\nexit 0\n')
            panel.chmod(0o755)
            (root / 'plan.json').write_text(json.dumps({'files': [str(root / p) for p in list(originals) + new_files]}))
            script = root / 'run.sh'
            script.write_text('''source "$INSTALLER"
INSTALL_DIR="$TEST_ROOT"
CONFIG_FILE="$TEST_ROOT/config.yaml"
BACKUP_DIR="$TEST_ROOT/backups"
WEBSITE_CONFIG_DIR="$TEST_ROOT/site"
WEBSITE_ROOT="$TEST_ROOT/html"
WEBSITE_UNIT_DIR="$TEST_ROOT/units"
check_root() { :; }
detect_os() { PKG_MGR=yum; }
flock() { :; }
sleep() { :; }
confirm() { return 0; }
install_website_deps() { [[ "$FAIL" != dependency ]]; }
setup_website_selinux() { :; }
website_tool() {
 case "$1" in
 prepare)
  if [[ "$VMESS" == 1 ]]; then [[ "$3" == site.example.com && "$4" == 2,3 ]] || return 1; fi
  [[ "$FAIL" != preflight ]] || return 1
  [[ "$FAIL" != final-preflight || "$2" != */final ]] || return 1
  mkdir -p "$2"
  cp "$TEST_ROOT/plan.json" "$2/plan.json"
  if [[ "$FAIL" == changed && "$2" == */final ]]; then
   jq '.changed=true' "$TEST_ROOT/plan.json" > "$2/plan.json"
  fi;;
 apply)
  echo changed > "$TEST_ROOT/panel.db"
  [[ "$FAIL" != apply ]] || return 1
  while IFS= read -r file; do
   mkdir -p "$(dirname "$file")"
   echo changed > "$file"
  done < <(jq -r '.files[] | select(endswith("config.yaml") | not)' "$TEST_ROOT/plan.json");;
 health) [[ "$FAIL" != health ]];;
 esac
}
systemctl() {
 local op="$1" svc="${@: -1}"
 echo "$*" >> "$TEST_ROOT/events"
 case "$op" in
 show) echo loaded;;
 is-active) [[ -f "$TEST_ROOT/states/$svc" ]];;
 is-enabled) [[ -f "$TEST_ROOT/enabled/$svc" ]];;
 stop)
  if [[ "$FAIL" == stop && ! -f "$TEST_ROOT/failed-once" ]]; then touch "$TEST_ROOT/failed-once"; return 1; fi
  shift
  for svc in "$@"; do rm -f "$TEST_ROOT/states/$svc"; done;;
 start)
  if [[ "$FAIL" == start && "$svc" == proxy-panel-web || "$FAIL" == watcher && "$svc" == proxy-panel-web-cert.path ]]; then return 1; fi
  touch "$TEST_ROOT/states/$svc";;
 enable) shift; for svc in "$@"; do touch "$TEST_ROOT/enabled/$svc"; done;;
 disable) shift; for svc in "$@"; do rm -f "$TEST_ROOT/enabled/$svc"; done;;
 esac
}
if [[ "$OFFER" == 1 ]]; then offer_website_setup
elif [[ "$VMESS" == 1 ]]; then do_website website setup site.example.com --vmess 2,3
else do_website website setup; fi
''')
            env = {**os.environ, 'TEST_ROOT': temp, 'INSTALLER': str(INSTALLER), 'FAIL': failure, 'OFFER': '1' if offer else '0', 'VMESS': '1' if vmess else '0'}
            result = subprocess.run(['bash', str(script)], env=env, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=25)
            self.assertEqual(result.returncode == 0, not failure or offer, result.stdout)
            if failure:
                for name, value in originals.items():
                    self.assertEqual((root / name).read_bytes(), value, name + result.stdout)
                for name in new_files:
                    self.assertFalse((root / name).exists(), name + result.stdout)
                self.assertEqual(sorted(p.name for p in (root / 'states').iterdir()), ['proxy-panel', 'xray'], result.stdout)
                self.assertEqual(list((root / 'enabled').iterdir()), [], result.stdout)
            else:
                self.assertEqual((root / 'config.yaml').read_bytes(), originals['config.yaml'])
                self.assertTrue((root / 'site/nginx.conf').exists(), result.stdout)
                self.assertTrue((root / 'states/proxy-panel-web').exists(), result.stdout)
                self.assertTrue((root / 'states/proxy-panel-web-cert.path').exists(), result.stdout)
            if failure in ('dependency', 'preflight', 'capability'):
                events = (root / 'events').read_text() if (root / 'events').exists() else ''
                self.assertNotIn('stop ', events, result.stdout)

    def test_success(self): self.run_case()
    def test_vmess_arguments_reach_both_preflight_passes(self): self.run_case(vmess=True)
    def test_vmess_old_binary_fails_before_stopping_services(self): self.run_case('capability', vmess=True)
    def test_vmess_health_failure_restores_originals(self): self.run_case('health', vmess=True)
    def test_dependency_failure_keeps_proxy(self): self.run_case('dependency')
    def test_preflight_failure_keeps_proxy(self): self.run_case('preflight')
    def test_stop_failure_recovers(self): self.run_case('stop')
    def test_final_preflight_failure_recovers(self): self.run_case('final-preflight')
    def test_concurrent_change_recovers(self): self.run_case('changed')
    def test_partial_apply_failure_recovers(self): self.run_case('apply')
    def test_nginx_start_failure_recovers(self): self.run_case('start')
    def test_cert_watcher_start_failure_recovers(self): self.run_case('watcher')
    def test_health_failure_recovers(self): self.run_case('health')
    def test_update_offer_does_not_disable_transaction_errors(self): self.run_case('apply', offer=True)


if __name__ == '__main__':
    unittest.main()
