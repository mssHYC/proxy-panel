"""Exercise the website helper shipped inside the standalone management script."""
import contextlib
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
                        '-subj', '/CN=site.example.com', '-addext',
                        'subjectAltName=DNS:site.example.com,DNS:cdn.example.com,DNS:archive.example.com',
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

    def prepare(self, website_domain='', action='setup', *args):
        stage = self.root / 'stage'
        tool.prepare(str(self.config), str(self.root), self.confdir, self.webroot, self.units, stage, website_domain, action, *args)
        return stage, json.loads((stage / 'plan.json').read_text())

    def standalone(self):
        with contextlib.closing(sqlite3.connect(self.db)) as db, db:
            db.execute('DELETE FROM nodes')
        cfg = json.loads(self.config.read_text())
        cfg['server'].update(tls=True, cert=str(self.cert), key=str(self.key))
        # A website must work with no Xray installed or configured.
        cfg.pop('kernel')
        self.config.write_text(json.dumps(cfg))
        self.stack.enter_context(patch.object(tool, 'check_standalone_ports'))

    def test_standalone_site_uses_panel_certificate_without_touching_proxy_data(self):
        self.standalone()
        before = {p: p.read_bytes() for p in (self.db, self.kernel, self.config)}
        stage, plan = self.prepare('site.example.com')
        self.assertEqual(plan['mode'], 'standalone')
        self.assertNotIn(str(self.db), plan['files'])
        self.assertNotIn(str(self.kernel), plan['files'])
        tool.apply(stage)
        conf = (self.confdir / 'nginx.conf').read_text()
        self.assertNotIn('proxy_pass', conf)
        self.assertIn('server_name site.example.com;', conf)
        self.assertIn('ssl_reject_handshake on;', conf)
        for path, contents in before.items():
            self.assertEqual(path.read_bytes(), contents)
        (self.webroot / 'index.html').write_text('custom-site')
        stage, repeated = self.prepare()
        tool.apply(stage)
        self.assertEqual(repeated['domains'], ['site.example.com'])
        self.assertEqual((self.webroot / 'index.html').read_text(), 'custom-site')

    def test_standalone_site_accepts_domain_certificate_without_panel_tls(self):
        self.standalone()
        cfg = json.loads(self.config.read_text())
        cfg['server'] = {'port': 13888}
        self.config.write_text(json.dumps(cfg))
        certdir = self.root / 'certs'
        certdir.mkdir()
        (certdir / 'site.example.com.crt').write_bytes(self.cert.read_bytes())
        (certdir / 'site.example.com.key').write_bytes(self.key.read_bytes())
        _, plan = self.prepare('site.example.com')
        self.assertEqual(plan['cert'], str(certdir / 'site.example.com.crt'))

    def test_standalone_missing_domain_or_certificate_is_actionable(self):
        self.standalone()
        cfg = json.loads(self.config.read_text())
        cfg['server'] = {'port': 13888}
        self.config.write_text(json.dumps(cfg))
        with self.assertRaisesRegex(ValueError, '域名'):
            self.prepare()
        with self.assertRaisesRegex(ValueError, '证书'):
            self.prepare('site.example.com')
        self.assertFalse(self.confdir.exists())

    def test_standalone_rejects_certificate_for_wrong_domain(self):
        self.standalone()
        with self.assertRaisesRegex(ValueError, '证书'):
            self.prepare('wrong.example.com')
        self.assertFalse(self.confdir.exists())

    def test_standalone_stays_available_with_hysteria2_on_udp_443(self):
        self.standalone()
        with contextlib.closing(sqlite3.connect(self.db)) as db, db:
            db.execute('INSERT INTO nodes VALUES (2,"site.example.com","hysteria2","","{}",1,443,"sing-box",NULL)')
        _, plan = self.prepare()
        self.assertEqual(plan['mode'], 'standalone')

    def test_existing_proxy_site_does_not_silently_drop_route_if_node_disappears(self):
        stage, _ = self.prepare()
        tool.apply(stage)
        with contextlib.closing(sqlite3.connect(self.db)) as db, db:
            db.execute('DELETE FROM nodes')
        with self.assertRaisesRegex(ValueError, '节点'):
            self.prepare()

    def node_settings(self):
        with contextlib.closing(sqlite3.connect(self.db)) as db, db:
            return json.loads(db.execute('SELECT settings FROM nodes WHERE id=1').fetchone()[0])

    def enable_reality_frontend(self):
        stage, _ = self.prepare()
        tool.apply(stage)
        path = self.confdir / 'managed.json'
        state = json.loads(path.read_text())
        front = {'node_id': 2, 'sni': 'target.example.com', 'listen': '127.0.0.2:443',
                 'web_listen': '127.0.0.1:9443', 'stream_module': ''}
        state['reality_frontend'] = front
        path.write_text(json.dumps(state))
        settings = {'security': 'reality', 'listen': '127.0.0.2', 'sni': front['sni']}
        with contextlib.closing(sqlite3.connect(self.db)) as db, db:
            db.execute('INSERT INTO nodes VALUES (2,?,?,?,?,1,443,"xray",NULL)',
                       ('site.example.com', 'vless', 'tcp', json.dumps(settings)))
        kernel = json.loads(self.kernel.read_text())
        kernel['inbounds'].append({'tag': 'node-2', 'listen': '127.0.0.2', 'port': 443, 'protocol': 'vless',
                                   'streamSettings': {'network': 'tcp', 'security': 'reality',
                                                      'realitySettings': {'serverNames': [front['sni']]}}})
        self.kernel.write_text(json.dumps(kernel))
        return front

    def add_independent_reality(self, website_domain='', sni='target.example.com'):
        stage, _ = self.prepare(website_domain)
        tool.apply(stage)
        settings = {'security': 'reality', 'sni': sni, 'private_key': 'test-only'}
        with contextlib.closing(sqlite3.connect(self.db)) as db, db:
            db.execute('INSERT INTO nodes VALUES (2,?,?,?,?,1,1279,"xray",NULL)',
                       ('site.example.com', 'vless', 'tcp', json.dumps(settings)))
        kernel = json.loads(self.kernel.read_text())
        kernel['inbounds'].append({'tag': 'node-2', 'listen': '0.0.0.0', 'port': 1279, 'protocol': 'vless',
                                   'streamSettings': {'network': 'tcp', 'security': 'reality',
                                                      'realitySettings': {'serverNames': [sni]}}})
        self.kernel.write_text(json.dumps(kernel))

    def test_share_and_unshare_use_the_existing_real_node_and_restore_it(self):
        self.add_independent_reality('archive.example.com')
        with contextlib.closing(sqlite3.connect(self.db)) as db, db:
            before = db.execute('SELECT port,settings FROM nodes WHERE id=2').fetchone()
        original_kernel = json.loads(self.kernel.read_bytes())
        with patch.object(tool, 'check_standalone_ports'), patch.object(tool, 'run', return_value=''):
            stage, _ = self.prepare('', 'share-reality', '2')
            tool.apply(stage)
            state = (self.confdir / 'managed.json').read_text()
            self.assertNotIn('test-only', state)
            self.assertIn('archive.example.com', json.loads(state)['domains'])
            self.assertIn('server_name archive.example.com ', (self.confdir / 'nginx.conf').read_text())
            self.assertIn('target.example.com 127.0.0.2:443;', (self.confdir / 'nginx.conf').read_text())
            with contextlib.closing(sqlite3.connect(self.db)) as db, db:
                port, settings = db.execute('SELECT port,settings FROM nodes WHERE id=2').fetchone()
                self.assertEqual((port, json.loads(settings)['listen']), (443,'127.0.0.2'))
                self.assertEqual(json.loads(settings)['private_key'], 'test-only')
            stage, _ = self.prepare('', 'unshare-reality')
            tool.apply(stage)
            with contextlib.closing(sqlite3.connect(self.db)) as db, db:
                port, settings = db.execute('SELECT port,settings FROM nodes WHERE id=2').fetchone()
                self.assertEqual((port,json.loads(settings)), (before[0],json.loads(before[1])))
            self.assertEqual(json.loads(self.kernel.read_bytes()), original_kernel)
            state = json.loads((self.confdir / 'managed.json').read_bytes())
            self.assertNotIn('reality_frontend', state)
            self.assertIn('archive.example.com', state['domains'])
            self.assertIn('server_name archive.example.com ', (self.confdir / 'nginx.conf').read_text())

    def test_share_rejects_reality_sni_reserved_by_existing_website_alias(self):
        self.add_independent_reality('archive.example.com', 'ARCHIVE.example.com.')
        files = (self.db, self.kernel, self.confdir / 'managed.json', self.confdir / 'nginx.conf')
        before = [path.read_bytes() for path in files]
        with self.assertRaisesRegex(ValueError, 'SNI'):
            self.prepare('', 'share-reality', '2')
        self.assertEqual([path.read_bytes() for path in files], before)

    def test_share_rejects_target_with_more_than_one_sni(self):
        self.add_independent_reality()
        cfg = json.loads(self.kernel.read_bytes())
        cfg['inbounds'][-1]['streamSettings']['realitySettings']['serverNames'].append('cdn.example.com')
        self.kernel.write_text(json.dumps(cfg))
        with self.assertRaisesRegex(ValueError, 'SNI'):
            self.prepare('', 'share-reality', '2')
        self.assertNotIn('reality_frontend', json.loads((self.confdir/'managed.json').read_bytes()))

    def test_share_uses_the_same_server_names_alias_as_xray(self):
        self.add_independent_reality()
        with contextlib.closing(sqlite3.connect(self.db)) as db, db:
            settings = json.loads(db.execute('SELECT settings FROM nodes WHERE id=2').fetchone()[0])
            settings.update(server_names='old invalid scalar', serverNames=[settings.pop('sni')])
            db.execute('UPDATE nodes SET settings=? WHERE id=2', (json.dumps(settings),))
        with patch.object(tool, 'check_standalone_ports'), patch.object(tool, 'run', return_value=''):
            stage, plan = self.prepare('', 'share-reality', '2')
            tool.apply(stage)
        self.assertEqual(plan['reality_frontend']['sni'], 'target.example.com')

    def test_share_detects_racing_reality_edit_before_applying(self):
        self.add_independent_reality()
        with patch.object(tool, 'check_standalone_ports'), patch.object(tool, 'run', return_value=''):
            stage, _ = self.prepare('', 'share-reality', '2')
        with contextlib.closing(sqlite3.connect(self.db)) as db, db:
            db.execute('UPDATE nodes SET port=9922 WHERE id=2')
        with self.assertRaisesRegex(ValueError, '预检'):
            tool.apply(stage)
        self.assertNotIn('reality_frontend', json.loads((self.confdir/'managed.json').read_bytes()))

    def test_rebind_requires_old_id_to_be_gone_and_the_new_id_to_match(self):
        stage, _ = self.prepare('archive.example.com')
        tool.apply(stage)
        manifest_path = self.confdir / 'managed.json'
        legacy = json.loads(manifest_path.read_text())
        del legacy['mode']
        manifest_path.write_text(json.dumps(legacy))
        with self.assertRaisesRegex(ValueError, '仍存在'):
            self.prepare('', 'rebind', '4')
        with contextlib.closing(sqlite3.connect(self.db)) as db, db:
            db.execute('UPDATE nodes SET id=4 WHERE id=1')
        state = json.loads(self.kernel.read_text())
        state['inbounds'][0]['tag'] = 'node-4'
        self.kernel.write_text(json.dumps(state))
        with self.assertRaisesRegex(ValueError, '指定节点'):
            self.prepare('', 'rebind', '5')
        with self.assertRaisesRegex(ValueError, '节点已更换'):
            self.prepare()
        stage, _ = self.prepare('', 'rebind', '4')
        tool.apply(stage)
        manifest = json.loads((self.confdir/'managed.json').read_text())
        self.assertEqual(manifest['node_id'], 4)
        self.assertEqual(manifest['mode'], 'proxy')
        self.assertNotIn('rebind_from', manifest)
        self.assertIn('archive.example.com', manifest['domains'])
        self.assertIn('server_name archive.example.com ', (self.confdir / 'nginx.conf').read_text())
        self.assertIn('127.0.0.1:443', (self.confdir/'nginx.conf').read_text())


    def test_repeated_setup_keeps_shared_443_and_both_nodes(self):
        front = self.enable_reality_frontend()
        before = json.loads(self.kernel.read_text())
        # This fixture checks DB/manifest/config round-tripping without REALITY credentials.
        with patch.object(tool, 'run', side_effect=lambda args: '' if args[0] in
                          (self.nginx, os.environ.get('XRAY_BINARY', '/fixture/xray')) else self.checked_run(args)):
            stage, _ = self.prepare()
        tool.apply(stage)
        new_state = json.loads((self.confdir / 'managed.json').read_text())
        self.assertEqual(new_state['reality_frontend'], front)
        self.assertEqual(new_state['listens'], ['192.0.2.10:443'])
        self.assertEqual(json.loads(self.kernel.read_text()), before)
        nginx = (self.confdir / 'nginx.conf').read_text()
        self.assertIn('listen 192.0.2.10:443;', nginx)
        self.assertIn('target.example.com 127.0.0.2:443;', nginx)
        self.assertIn('default 127.0.0.1:9443;', nginx)
        self.assertIn('listen 127.0.0.1:9443 ssl;', nginx)
        self.assertIn('proxy_pass https://127.0.0.1:443;', nginx)

    def test_extra_443_rejected_after_frontend_is_managed(self):
        self.enable_reality_frontend()
        with contextlib.closing(sqlite3.connect(self.db)) as db, db:
            db.execute('INSERT INTO nodes SELECT 3,host,protocol,transport,settings,enable,port,kernel_type,updated_at FROM nodes WHERE id=2')
            for addr in ('0.0.0.0', '::', '::ffff:192.0.2.10', '127.0.0.1', '127.0.0.2'):
                with self.subTest(addr=addr):
                    db.execute('UPDATE nodes SET settings=? WHERE id=3', (json.dumps({'listen': addr}),))
                    db.commit()
                    with self.assertRaisesRegex(ValueError, '443 节点'):
                        self.prepare()

    def test_disjoint_443_bindings_survive_share_and_unshare(self):
        self.add_independent_reality()
        with contextlib.closing(sqlite3.connect(self.db)) as db, db:
            db.execute('INSERT INTO nodes VALUES (3, "other.example.com", "vless","tcp",?,1,443,"xray",NULL)',
                       (json.dumps({'listen': '127.0.0.3'}),))
        other = {'tag': 'node-3', 'listen': '127.0.0.3', 'port': 443, 'protocol': 'vless'}
        kernel = json.loads(self.kernel.read_text())
        kernel['inbounds'].append(other)
        self.kernel.write_text(json.dumps(kernel))
        with patch.object(tool, 'check_standalone_ports'), patch.object(tool, 'run', return_value=''):
            stage, _ = self.prepare('', 'share-reality', '2')
            tool.apply(stage)
            kernel = json.loads(self.kernel.read_text())
            self.assertIn(other, kernel['inbounds'])
            kernel['inbounds'][-1]['listen'] = '::'
            self.kernel.write_text(json.dumps(kernel))
            with self.assertRaisesRegex(ValueError, '443 节点'):
                self.prepare('', 'unshare-reality')
            kernel['inbounds'][-1] = other
            self.kernel.write_text(json.dumps(kernel))
            stage, _ = self.prepare('', 'unshare-reality')
            tool.apply(stage)
        self.assertIn(other, json.loads(self.kernel.read_text())['inbounds'])

    def test_rebind_moves_temporary_loopback_to_shared_website_backend(self):
        front = self.enable_reality_frontend()
        with contextlib.closing(sqlite3.connect(self.db)) as db, db:
            settings = json.loads(db.execute('SELECT settings FROM nodes WHERE id=1').fetchone()[0])
            settings.update(listen='127.0.0.3', path='/replacement')
            db.execute('UPDATE nodes SET id=4,settings=? WHERE id=1', (json.dumps(settings),))
        config = json.loads(self.kernel.read_text())
        inbound = config['inbounds'][0]
        inbound['tag'] = 'node-4'
        inbound['streamSettings']['httpupgradeSettings']['path'] = '/replacement'
        self.kernel.write_text(json.dumps(config))
        with self.assertRaisesRegex(ValueError, '运行监听'):
            self.prepare('', 'rebind', '4')
        inbound['listen'] = '127.0.0.3'
        self.kernel.write_text(json.dumps(config))
        with patch.object(tool, 'run', return_value=''):
            stage, plan = self.prepare('', 'rebind', '4')
        tool.apply(stage)
        self.assertEqual(plan['reality_frontend'], front)
        self.assertEqual(plan['listen_before'], '0.0.0.0')
        self.assertEqual(plan['listens'], ['192.0.2.10:443'])
        self.assertEqual(json.loads(self.kernel.read_text())['inbounds'][0]['listen'], '127.0.0.1')
        with contextlib.closing(sqlite3.connect(self.db)) as db:
            settings = json.loads(db.execute('SELECT settings FROM nodes WHERE id=4').fetchone()[0])
            self.assertEqual(settings['listen'], '127.0.0.1')
        self.assertIn('location = "/replacement"', (self.confdir / 'nginx.conf').read_text())

    def test_stale_reality_kernel_listener_is_rejected(self):
        self.enable_reality_frontend()
        kernel = json.loads(self.kernel.read_text())
        kernel['inbounds'][1]['listen'] = '0.0.0.0'
        self.kernel.write_text(json.dumps(kernel))
        with self.assertRaisesRegex(ValueError, '托管 Reality 入口'):
            self.prepare()

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

    @unittest.skipUnless(os.environ.get('NGINX_BINARY'), 'set NGINX_BINARY for standalone HTTPS integration')
    def test_real_standalone_nginx_serves_https_without_proxy_backend(self):
        self.standalone()
        _, plan = self.prepare()
        self.root.chmod(0o755)
        self.webroot.mkdir()
        (self.webroot / 'index.html').write_text('standalone-website-ok')
        with socket.socket() as probe:
            probe.bind(('127.0.0.1', 0))
            port = probe.getsockname()[1]
        plan['listens'] = [f'127.0.0.1:{port}']
        config = self.root / 'standalone.conf'
        config.write_text(tool.nginx_config(plan, self.webroot).replace('pid /run/proxy-panel-web.pid;', f'pid {self.root}/nginx.pid;'))
        log = (self.root / 'nginx.log').open('w+')
        self.addCleanup(log.close)
        process = subprocess.Popen([self.nginx, '-p', str(self.root), '-c', str(config), '-g', 'daemon off;'], stdout=log, stderr=log)
        self.addCleanup(lambda: (process.terminate(), process.wait(timeout=5)) if process.poll() is None else None)
        for _ in range(40):
            if process.poll() is not None:
                log.seek(0)
                self.fail(log.read())
            try:
                with socket.create_connection(('127.0.0.1', port), timeout=.1): break
            except OSError: time.sleep(.05)
        state = self.root / 'standalone.json'
        state.write_text(json.dumps(plan))
        tool.health(state)
        ctx = ssl.create_default_context(cafile=str(self.cert))
        for path, expected in [('/', b'200 OK'), ('/api', b'404 Not Found')]:
            with socket.create_connection(('127.0.0.1', port), timeout=3) as sock:
                with ctx.wrap_socket(sock, server_hostname='site.example.com') as conn:
                    conn.sendall(f'GET {path} HTTP/1.1\r\nHost: site.example.com\r\nConnection: close\r\n\r\n'.encode())
                    response = conn.makefile('rb').read()
                    self.assertIn(expected, response)
                    if path == '/': self.assertIn(b'standalone-website-ok', response)
        with socket.create_connection(('127.0.0.1', port), timeout=3) as sock:
            with self.assertRaises(ssl.SSLError):
                ctx.wrap_socket(sock, server_hostname='wrong.example.com')

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


    @unittest.skipUnless(os.environ.get('NGINX_BINARY') and os.environ.get('XRAY_BINARY'),
                         'set NGINX_BINARY and XRAY_BINARY for the authenticated shared-port test')
    def test_real_shared_port_authenticates_reality_and_serves_httpupgrade(self):
        # All services, including the REALITY TLS target and client destination, are local.
        xray = os.environ['XRAY_BINARY']

        class Site(BaseHTTPRequestHandler):
            def do_GET(self):
                self.send_response(200)
                self.end_headers()
                self.wfile.write(b'through-proxy')

            def handle(self):
                try:
                    super().handle()
                except ssl.SSLError:
                    # Reality's unauthenticated target connections may close mid-handshake.
                    if not isinstance(self.connection, ssl.SSLSocket):
                        raise

            def log_message(self, *args):
                pass

        def server(tls=False):
            srv = ThreadingHTTPServer(('127.0.0.1', 0), Site)
            if tls:
                ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
                ctx.load_cert_chain(self.cert, self.key)
                ctx.set_alpn_protocols(['h2', 'http/1.1'])
                srv.socket = ctx.wrap_socket(srv.socket, server_side=True, do_handshake_on_connect=False)
            threading.Thread(target=srv.serve_forever, daemon=True).start()
            self.addCleanup(srv.server_close)
            self.addCleanup(srv.shutdown)
            return srv.server_port

        target, origin = server(tls=True), server()
        with contextlib.ExitStack() as allocated:
            ports = []
            for _ in range(6):
                sock = allocated.enter_context(socket.socket())
                sock.bind(('127.0.0.1', 0))
                ports.append(sock.getsockname()[1])
        public, web, backend, reality, reality_socks, http_socks = ports

        def start(args, port):
            log = tempfile.TemporaryFile()
            self.addCleanup(log.close)
            proc = subprocess.Popen(args, stdout=log, stderr=subprocess.STDOUT)
            self.addCleanup(lambda: (proc.terminate(), proc.wait(timeout=5)) if proc.poll() is None else None)
            for _ in range(60):
                if proc.poll() is not None:
                    log.seek(0)
                    self.fail(log.read().decode())
                try:
                    with socket.create_connection(('127.0.0.1', port), .2): return
                except OSError: time.sleep(.05)
            self.fail('local listener did not start: ' + str(port))

        self.root.chmod(0o755)
        self.webroot.mkdir()
        (self.webroot/'index.html').write_text('website')
        ctx = self.original_core['inbounds'][0]
        uuid = ctx['settings']['clients'][0]['id']
        ctx.update(listen='127.0.0.1', port=backend)
        keys = dict(line.split(': ', 1) for line in
                    subprocess.check_output([xray, 'x25519'], text=True).splitlines() if ': ' in line)
        reality_inbound = {'listen': '127.0.0.1', 'port': reality, 'protocol': 'vless',
            'settings': {'decryption':'none', 'clients':[{'id':uuid, 'flow':'xtls-rprx-vision'}]},
            'streamSettings': {'network':'tcp', 'security':'reality', 'realitySettings': {
                'target': f'127.0.0.1:{target}', 'serverNames':['site.example.com'],
                'privateKey':keys['PrivateKey'], 'shortIds':['12345678']}}}
        self.original_core['inbounds'].append(reality_inbound)
        # The real server permits the test's local HTTP origin as its only destination.
        self.original_core['outbounds'][0]['settings'] = {'finalRules': [
            {'action': 'allow', 'network': 'tcp', 'ip': ['127.0.0.1/32'], 'port': origin},
            {'action': 'block'},
        ]}
        self.kernel.write_text(json.dumps(self.original_core))
        start([xray, 'run', '-config', str(self.kernel)], backend)
        plan = dict(mode='proxy', domains=['cdn.example.com'], host='cdn.example.com', sni='cdn.example.com',
                    path='/csda', cert=str(self.cert), key=str(self.key), ca=str(self.cert), listens=[f'127.0.0.1:{public}'])
        plan['reality_frontend'] = dict(node_id=2, sni='site.example.com', listen=f'127.0.0.1:{reality}',
                                       web_listen=f'127.0.0.1:{web}', stream_module=next((p for p in tool.STREAM_MODULES if Path(p).is_file()), ''))
        c = tool.nginx_config(plan, self.webroot).replace('/run/proxy-panel-web.pid', str(self.root/'nginx.pid'))
        c = c.replace('https://127.0.0.1:443;', f'https://127.0.0.1:{backend};')
        conf = self.root/'nginx.conf'
        conf.write_text(c)
        start([self.nginx, '-c', str(conf), '-g', 'daemon off;'], public)
        state = self.root/'managed.json'
        state.write_text(json.dumps(plan))
        tool.health(state)
        reality_stream = {'network': 'tcp', 'security': 'reality', 'realitySettings': {
            'fingerprint': 'chrome', 'serverName': 'site.example.com',
            'shortId': '12345678', 'password': keys['Password (PublicKey)'],
        }}
        http_stream = {'network': 'httpupgrade', 'security': 'tls',
                       'httpupgradeSettings': {'path': '/csda', 'host': 'cdn.example.com'},
                       'tlsSettings': {'serverName': 'cdn.example.com', 'alpn': ['http/1.1'],
                                       'certificates': [{'usage': 'verify', 'certificateFile': str(self.cert)}]}}
        for stream, socks, flow in [(reality_stream, reality_socks, 'xtls-rprx-vision'),
                                    (http_stream, http_socks, '')]:
            with self.subTest(transport=stream['network']):
                server = {'address': '127.0.0.1', 'port': public, 'users': [
                    {'id': uuid, 'encryption': 'none', 'flow': flow},
                ]}
                client = {'inbounds': [{'listen': '127.0.0.1', 'port': socks, 'protocol': 'socks'}],
                          'outbounds': [{'protocol': 'vless', 'settings': {'vnext': [server]},
                                         'streamSettings': stream}]}
                path = self.root / (stream['network'] + '-client.json')
                path.write_text(json.dumps(client))
                start([xray, 'run', '-config', str(path)], socks)
                output = subprocess.check_output(['curl', '--noproxy', '', '--proxy', f'socks5://127.0.0.1:{socks}',
                                                  '--max-time', '10', '-fsS', f'http://127.0.0.1:{origin}/'], text=True)
                self.assertEqual(output, 'through-proxy')


class WebsitePortTests(unittest.TestCase):
    def test_conflicting_tcp_listener_fails_before_deployment(self):
        with socket.socket() as listener:
            listener.bind(('127.0.0.1', 0))
            listener.listen()
            address = f'127.0.0.1:{listener.getsockname()[1]}'
            with self.assertRaisesRegex(ValueError, '占用'):
                tool.check_standalone_ports([address], None)
        tool.check_standalone_ports([address], None)

    def test_repeat_setup_skips_only_active_managed_listeners(self):
        with socket.socket() as listener:
            listener.bind(('127.0.0.1', 0))
            listener.listen()
            address = f'127.0.0.1:{listener.getsockname()[1]}'
            managed = {'listens': [address]}
            with patch.object(tool.subprocess, 'run', return_value=types.SimpleNamespace(returncode=0)):
                tool.check_standalone_ports([address], managed)
                with self.assertRaisesRegex(ValueError, '占用'):
                    tool.check_standalone_ports([address], {'listens': []})
            with patch.object(tool.subprocess, 'run', return_value=types.SimpleNamespace(returncode=3)):
                with self.assertRaisesRegex(ValueError, '占用'):
                    tool.check_standalone_ports([address], managed)


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
    def run_case(self, failure='', offer=False, standalone=False):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            (root / 'states').mkdir()
            (root / 'enabled').mkdir()
            original_services = ['proxy-panel'] if standalone else ['proxy-panel', 'xray']
            for svc in original_services:
                (root / 'states' / svc).touch()
            originals = {'panel.db': b'old-db', 'xray.json': b'old-core', 'config.yaml': b'unchanged-listen'}
            new_files = ['site/nginx.conf', 'site/managed.json', 'html/index.html', 'units/proxy-panel-web.service',
                         'units/proxy-panel-web-reload.service', 'units/proxy-panel-web-cert.path']
            for name, value in originals.items():
                (root / name).write_bytes(value)
            panel = root / 'proxy-panel'
            panel.write_text('#!/bin/bash\nif [[ "$STANDALONE" == 1 && "$*" == *-check-update* ]]; then exit 1; fi\nexit 0\n')
            panel.chmod(0o755)
            (root / 'plan.json').write_text(json.dumps({'mode': 'standalone' if standalone else 'proxy',
                'files': [str(root / p) for p in ([] if standalone else list(originals)) + new_files]}))
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
  [[ "$FAIL" != preflight ]] || return 1
  [[ "$FAIL" != final-preflight || "$2" != */final ]] || return 1
  mkdir -p "$2"
  cp "$TEST_ROOT/plan.json" "$2/plan.json"
  if [[ "$FAIL" == changed && "$2" == */final ]]; then
   jq '.changed=true' "$TEST_ROOT/plan.json" > "$2/plan.json"
  fi;;
 apply)
  if [[ "$STANDALONE" != 1 ]]; then echo changed > "$TEST_ROOT/panel.db"; fi
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
if [[ "$OFFER" == 1 ]]; then offer_website_setup; else do_website website setup; fi
''')
            env = {**os.environ, 'TEST_ROOT': temp, 'INSTALLER': str(INSTALLER), 'FAIL': failure,
                   'OFFER': '1' if offer else '0', 'STANDALONE': '1' if standalone else '0'}
            result = subprocess.run(['bash', str(script)], env=env, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=25)
            self.assertEqual(result.returncode == 0, not failure or offer, result.stdout)
            if failure:
                for name, value in originals.items():
                    self.assertEqual((root / name).read_bytes(), value, name + result.stdout)
                for name in new_files:
                    self.assertFalse((root / name).exists(), name + result.stdout)
                self.assertEqual(sorted(p.name for p in (root / 'states').iterdir()), original_services, result.stdout)
                self.assertEqual(list((root / 'enabled').iterdir()), [], result.stdout)
            else:
                self.assertEqual((root / 'config.yaml').read_bytes(), originals['config.yaml'])
                self.assertTrue((root / 'site/nginx.conf').exists(), result.stdout)
                self.assertTrue((root / 'states/proxy-panel-web').exists(), result.stdout)
                self.assertTrue((root / 'states/proxy-panel-web-cert.path').exists(), result.stdout)
            if failure in ('dependency', 'preflight'):
                events = (root / 'events').read_text() if (root / 'events').exists() else ''
                self.assertNotIn('stop ', events, result.stdout)
            if standalone:
                for name, value in originals.items():
                    self.assertEqual((root / name).read_bytes(), value, result.stdout)
                events = (root / 'events').read_text()
                for svc in ('proxy-panel', 'xray', 'sing-box'):
                    self.assertNotIn('stop ' + svc + '\n', events)
                    self.assertNotIn('start ' + svc + '\n', events)

    def test_success(self): self.run_case()
    def test_standalone_success_without_xray(self): self.run_case(standalone=True)
    def test_standalone_nginx_failure_keeps_panel_running(self): self.run_case('start', standalone=True)
    def test_standalone_health_failure_keeps_panel_running(self): self.run_case('health', standalone=True)
    def test_standalone_partial_apply_recovers(self): self.run_case('apply', standalone=True)
    def test_standalone_watcher_failure_recovers(self): self.run_case('watcher', standalone=True)
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
