package kernel

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Opt in with XRAY_BINARY and SINGBOX_BINARY pointing at official binaries.
// This uses unprivileged loopback listeners and Cloudflare 1.1.1.1 for a positive
// outbound control. It never touches system services or a production config.
func TestCoreIntegration(t *testing.T) {
	xray, singbox := os.Getenv("XRAY_BINARY"), os.Getenv("SINGBOX_BINARY")
	if xray == "" || singbox == "" {
		t.Skip("set XRAY_BINARY and SINGBOX_BINARY to run real-core checks")
	}
	dir := t.TempDir()
	certSource := httptest.NewTLSServer(http.NotFoundHandler())
	defer certSource.Close()
	cert := certSource.TLS.Certificates[0]
	certPath, keyPath := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	key, err := x509.MarshalPKCS8PrivateKey(cert.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	write := func(path string, data []byte) {
		t.Helper()
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[0]}))
	write(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key}))
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "local website") }))
	defer site.Close()
	siteURL, _ := url.Parse(site.URL)
	sitePort, _ := strconv.Atoi(siteURL.Port())
	apiPort, proxyPort, socksPort, sbPort := freePort(t), freePort(t), freePort(t), freePort(t)
	settings := map[string]interface{}{"listen": "127.0.0.1", "security": "tls", "sni": "example.com", "cert_path": certPath, "key_path": keyPath, "flow": "xtls-rprx-vision", "fallback_port": sitePort}
	node := NodeConfig{ID: 1, Tag: "node-1", Port: proxyPort, Protocol: "vless", Transport: "tcp", Settings: settings}
	users := []UserConfig{{UUID: "00000000-0000-0000-0000-000000000001", Email: "integration"}}
	xe := NewXrayEngine(xray, filepath.Join(dir, "xray.json"), apiPort)
	data, err := xe.GenerateConfig([]NodeConfig{node}, users)
	if err != nil {
		t.Fatal(err)
	}
	var debugConfig map[string]interface{}
	json.Unmarshal(data, &debugConfig)
	debugConfig["log"] = map[string]interface{}{"loglevel": "debug"}
	data, _ = json.Marshal(debugConfig)
	write(xe.configPath, data)
	if out, err := exec.Command(xray, "run", "-test", "-config", xe.configPath).CombinedOutput(); err != nil {
		t.Fatalf("Xray config: %s: %v", out, err)
	}
	startCore(t, dir, xray, "run", "-config", xe.configPath)
	waitPort(t, proxyPort)
	roots := x509.NewCertPool()
	roots.AddCert(certSource.Certificate())
	tlsCfg := &tls.Config{RootCAs: roots, ServerName: "example.com", NextProtos: []string{"http/1.1"}}
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 3 * time.Second}, "tcp", fmt.Sprintf("127.0.0.1:%d", proxyPort), tlsCfg)
	if err != nil {
		t.Fatal(err)
	}
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	fmt.Fprint(conn, "GET / HTTP/1.1\r\nHost: example.com\r\nConnection: close\r\n\r\n")
	response, err := io.ReadAll(conn)
	conn.Close()
	if err != nil || !strings.Contains(string(response), "local website") {
		t.Fatalf("website fallback: %q %v", response, err)
	}
	bad := tlsCfg.Clone()
	bad.ServerName = "unknown.invalid"
	bad.InsecureSkipVerify = true // test server rejection, not client certificate checking
	badConn, err := tls.DialWithDialer(&net.Dialer{Timeout: 3 * time.Second}, "tcp", fmt.Sprintf("127.0.0.1:%d", proxyPort), bad)
	if err == nil {
		// TLS 1.3 can surface a server handshake alert on the first read.
		badConn.SetDeadline(time.Now().Add(3 * time.Second))
		fmt.Fprint(badConn, "GET / HTTP/1.1\r\nHost: unknown.invalid\r\nConnection: close\r\n\r\n")
		reply, readErr := io.ReadAll(badConn)
		badConn.Close()
		if readErr == nil || strings.Contains(string(reply), "local website") {
			t.Fatalf("unknown SNI accepted: %q %v", reply, readErr)
		}
	}
	// Trust only the local certificate fixture for this client.
	client := map[string]interface{}{
		"inbounds": []interface{}{map[string]interface{}{
			"listen": "127.0.0.1", "port": socksPort, "protocol": "socks",
			"settings": map[string]interface{}{"auth": "noauth"},
		}},
		"outbounds": []interface{}{map[string]interface{}{
			"protocol": "vless",
			"settings": map[string]interface{}{
				"vnext": []interface{}{map[string]interface{}{
					"address": "127.0.0.1", "port": proxyPort,
					"users": []interface{}{map[string]interface{}{
						"id": users[0].UUID, "encryption": "none", "flow": "xtls-rprx-vision",
					}},
				}},
			},
			"streamSettings": map[string]interface{}{
				"network": "tcp", "security": "tls",
				"tlsSettings": map[string]interface{}{
					"serverName": "example.com", "disableSystemRoot": true,
					"certificates": []interface{}{map[string]interface{}{
						"certificateFile": certPath, "usage": "verify",
					}},
					"fingerprint": "chrome", "alpn": []string{"http/1.1"},
				},
			},
		}},
	}
	client["log"] = map[string]interface{}{"loglevel": "debug"}
	clientData, _ := json.Marshal(client)
	clientPath := filepath.Join(dir, "client.json")
	write(clientPath, clientData)
	startCore(t, dir, xray, "run", "-config", clientPath)
	waitPort(t, socksPort)
	proxyURL, _ := url.Parse(fmt.Sprintf("socks5://127.0.0.1:%d", socksPort))
	transport := &http.Transport{Proxy: http.ProxyURL(proxyURL)}
	defer transport.CloseIdleConnections()
	httpClient := &http.Client{Transport: transport, Timeout: 12 * time.Second}
	resp, err := httpClient.Get("https://1.1.1.1/cdn-cgi/trace")
	if err != nil {
		t.Fatalf("positive external proxy control failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("external control status: %d", resp.StatusCode)
	}
	httpClient.Timeout = 3 * time.Second
	for _, destination := range []string{site.URL, "http://localhost:" + siteURL.Port(), fmt.Sprintf("http://127.0.0.1:%d", apiPort), "http://[::1]:" + siteURL.Port()} {
		resp, err := httpClient.Get(destination)
		if err == nil {
			resp.Body.Close()
			t.Fatalf("proxy reached management destination %s", destination)
		}
	}
	if _, err := xe.GetTrafficStats(); err != nil {
		t.Fatal(err)
	}
	// Check the actual generated sing-box config and its controller authentication.
	sbSettings := map[string]interface{}{"listen": "127.0.0.1"}
	se := NewSingboxEngine(singbox, filepath.Join(dir, "singbox.json"), sbPort)
	sbProxyPort := freePort(t)
	sbData, err := se.GenerateConfig([]NodeConfig{{ID: 1, Tag: "node-1", Port: sbProxyPort, Protocol: "vless", Transport: "tcp", Settings: sbSettings}}, users)
	if err != nil {
		t.Fatal(err)
	}
	write(se.configPath, sbData)
	if out, err := exec.Command(singbox, "check", "-c", se.configPath).CombinedOutput(); err != nil {
		t.Fatalf("sing-box config: %s: %v", out, err)
	}
	startCore(t, dir, singbox, "run", "-c", se.configPath)
	waitPort(t, sbPort)
	resp, err = http.Get(fmt.Sprintf("http://127.0.0.1:%d/connections", sbPort))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("controller status without token: %d", resp.StatusCode)
	}
	if _, err := se.GetTrafficStats(); err != nil {
		t.Fatal(err)
	}
	// Reuse the client transport with a second SOCKS listener for sing-box.
	sbSocksPort := freePort(t)
	client["inbounds"].([]interface{})[0].(map[string]interface{})["port"] = sbSocksPort
	outbound := client["outbounds"].([]interface{})[0].(map[string]interface{})
	outbound["streamSettings"] = map[string]interface{}{"network": "tcp"}
	remote := outbound["settings"].(map[string]interface{})["vnext"].([]interface{})[0].(map[string]interface{})
	remote["port"] = sbProxyPort
	delete(remote["users"].([]interface{})[0].(map[string]interface{}), "flow")
	sbClientData, _ := json.Marshal(client)
	sbClientPath := filepath.Join(dir, "sb-client.json")
	write(sbClientPath, sbClientData)
	startCore(t, dir, xray, "run", "-config", sbClientPath)
	waitPort(t, sbSocksPort)
	sbProxyURL, _ := url.Parse(fmt.Sprintf("socks5://127.0.0.1:%d", sbSocksPort))
	sbTransport := &http.Transport{Proxy: http.ProxyURL(sbProxyURL)}
	defer sbTransport.CloseIdleConnections()
	sbClient := &http.Client{Transport: sbTransport, Timeout: 12 * time.Second}
	resp, err = sbClient.Get("https://1.1.1.1/cdn-cgi/trace")
	if err != nil {
		t.Fatalf("sing-box external control: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("sing-box external control status: %d", resp.StatusCode)
	}
	sbClient.Timeout = 3 * time.Second
	for _, destination := range []string{site.URL, "http://localhost:" + siteURL.Port(), fmt.Sprintf("http://127.0.0.1:%d", sbPort), "http://[::1]:" + siteURL.Port()} {
		resp, err := sbClient.Get(destination)
		if err == nil {
			resp.Body.Close()
			t.Fatalf("sing-box reached management destination %s", destination)
		}
	}
	t.Log("Xray TLS website fallback, SNI rejection, Vision forwarding, API stats, private-IP/domain isolation; sing-box forwarding, private-IP/domain isolation and authenticated controller passed")
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}
func waitPort(t *testing.T, port int) {
	t.Helper()
	for i := 0; i < 100; i++ {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 100*time.Millisecond)
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("core did not listen on %d", port)
}
func startCore(t *testing.T, dir, binary string, args ...string) {
	t.Helper()
	log, err := os.CreateTemp(dir, "core-*.log")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, args...)
	cmd.Stdout = log
	cmd.Stderr = log
	if err := cmd.Start(); err != nil {
		log.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cmd.Process.Kill()
		cmd.Wait()
		log.Close()
		if t.Failed() {
			data, _ := os.ReadFile(log.Name())
			t.Log(string(data))
		}
	})
}
