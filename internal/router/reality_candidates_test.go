package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"proxy-panel/internal/config"
)

func TestRealityCandidateRoutesRequireLogin(t *testing.T) {
	r := Setup(&config.Config{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "", nil)
	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/api/nodes/reality-candidate-catalog"},
		{http.MethodPost, "/api/nodes/check-reality-candidates"},
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(route.method, route.path, nil))
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s = %d, expected 401", route.method, route.path, w.Code)
		}
	}
}
