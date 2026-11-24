package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRealityCandidatesRequest(t *testing.T) {
	h := NewNodeHandler(nil, nil)
	r := gin.New()
	r.POST("/candidates", h.CheckRealityCandidates)
	post := func(body string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("POST", "/candidates", strings.NewReader(body)))
		return w
	}
	for _, body := range []string{"", "{}", `{"hosts":[]}`, `{"hosts":[123]}`, `{"hosts":["bad"]}{}`, strings.Repeat(" ", 16<<10) + "{}"} {
		if got := post(body).Code; got != http.StatusBadRequest {
			t.Fatalf("malformed request got %d", got)
		}
	}
	w := post(`{"hosts":["localhost"]}`)
	var response struct{ Results []struct{ Status string } }
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil || w.Code != http.StatusOK || len(response.Results) != 1 || response.Results[0].Status != "invalid_host" {
		t.Fatalf("unexpected response: %d %s", w.Code, w.Body)
	}
	h.candidateBatch <- struct{}{}
	if got := post(`{"hosts":["localhost"]}`).Code; got != http.StatusTooManyRequests {
		t.Fatalf("concurrent request got %d", got)
	}
}
