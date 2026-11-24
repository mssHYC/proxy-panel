package service_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSharePreflightExplainsMissingNginxStream(t *testing.T) {
	f := newManagedRealityFixture(t, "0.0.0.0")
	if err := os.WriteFile(filepath.Join(f.root, "nginx"), []byte("#!/bin/sh\necho \"$NGINX_TEST_ERROR\" >&2\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		message string
		hint    bool
	}{
		{`unknown directive "stream"`, true},
		{`unknown directive "ssl_preread"`, true},
		{`unknown "ssl_preread_server_name" variable`, true},
		{`dlopen() "/usr/lib/nginx/modules/ngx_stream_module.so" failed (cannot open shared object file: No such file or directory)`, true},
		{`module "/usr/lib64/nginx/modules/ngx_stream_module.so" is not binary compatible`, true},
		{`host not found in upstream "example.com"`, false},
		{`"stream" directive is not allowed here`, false},
		{`dlopen() "/usr/lib/nginx/modules/ngx_stream_module.so" failed (Permission denied)`, false},
	} {
		t.Run(tc.message, func(t *testing.T) {
			t.Setenv("NGINX_TEST_ERROR", tc.message)
			req := httptest.NewRequest(http.MethodPut, "/nodes/41", strings.NewReader(`{"port":443,"share_website_443":true}`))
			req.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			f.api.ServeHTTP(response, req)
			var body struct{ Error string }
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || response.Code < 400 || !strings.Contains(body.Error, tc.message) {
				t.Fatalf("lost Nginx error: %d %s", response.Code, response.Body)
			}
			for _, command := range []string{"apt-get install libnginx-mod-stream", "dnf install nginx-mod-stream"} {
				if strings.Contains(body.Error, command) != tc.hint {
					t.Errorf("unexpected installation advice: %s", body.Error)
				}
			}
			f.verify(t, f.port, "0.0.0.0", "a.example.com")
		})
	}
}
