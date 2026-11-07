package view

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTMLTemplates(t *testing.T) {
	renderer, err := NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	for _, page := range []string{"auth", "about", "error"} {
		w := httptest.NewRecorder()
		data := map[string]any{"Title": "Тест", "CSRF": "test-token", "Error": "Тестовая ошибка"}
		if err := renderer.Render(w, 200, page, data); err != nil {
			t.Fatalf("%s: %v", page, err)
		}
		if w.Code != 200 || !strings.HasPrefix(strings.ToLower(w.Body.String()), "<!doctype html>") {
			t.Errorf("%s is not HTML", page)
		}
		if got := w.Header().Get("Content-Type"); !strings.Contains(got, "text/html") {
			t.Errorf("%s: content type %q", page, got)
		}
	}
	css, err := Stylesheet()
	if err != nil || !strings.Contains(string(css), ".site-header") {
		t.Fatal("styles not embedded", err)
	}
}