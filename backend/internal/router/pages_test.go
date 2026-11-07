package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/NorousS/kai-web-dev/backend/internal/repository"
	"github.com/NorousS/kai-web-dev/backend/internal/service"
	"github.com/gin-gonic/gin"
)

func TestServerRenderedPagesAndCSRF(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := service.New(repository.New(nil), "0123456789abcdefghij0123456789abcdefghij")
	app := New(s, nil, false)
	for _, tc := range []struct {
		path     string
		status   int
		contains string
	}{
		{"/login", http.StatusOK, "<form"},
		{"/register", http.StatusOK, "<form"},
		{"/projects", http.StatusSeeOther, ""},
		{"/static/style.css", http.StatusOK, ".site-header"},
	} {
		r := httptest.NewRecorder()
		app.ServeHTTP(r, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if r.Code != tc.status || !strings.Contains(r.Body.String(), tc.contains) {
			t.Fatalf("GET %s: status %d body %s", tc.path, r.Code, r.Body.String())
		}
		if tc.path == "/login" && !strings.Contains(r.Header().Get("Content-Type"), "text/html") {
			t.Fatal("login is not server-rendered HTML")
		}
	}
	r := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("email=x%40a.com&password=abc"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	app.ServeHTTP(r, req)
	if r.Code != http.StatusForbidden {
		t.Fatalf("missing CSRF should give 403, got %d", r.Code)
	}
}