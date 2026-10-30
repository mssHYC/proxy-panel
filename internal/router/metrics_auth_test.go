package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestMetricsAccess(t *testing.T) {
	token := strings.Repeat("a", 32)
	for _, tc := range []struct {
		name, token, auth, host string
		status                  int
	}{
		{"disabled", "", "", "panel.example.com", 404},
		{"missing", token, "", "panel.example.com", 401},
		{"wrong", token, "Bearer wrong", "panel.example.com", 401},
		{"authorized", token, "Bearer " + token, "panel.example.com", 200},
		{"domain guard", token, "Bearer " + token, "wrong.example.com", 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			r.Use(DomainGuard("panel.example.com"))
			registerMetrics(r, tc.token)
			req := httptest.NewRequest(http.MethodGet, "http://"+tc.host+"/metrics", nil)
			req.Header.Set("Authorization", tc.auth)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != tc.status {
				t.Fatalf("status=%d, want=%d", w.Code, tc.status)
			}
		})
	}
}
