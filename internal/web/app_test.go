package web_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"taskmanager/internal/web"
	"testing"
)

func TestHomePage(t *testing.T) {
	server := httptest.NewServer(web.New(nil, false))
	defer server.Close()
	response, err := http.Get(server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || len(body) == 0 {
		t.Fatalf("home: status %d, empty=%v", response.StatusCode, len(body) == 0)
	}
}
