package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestUrgency(t *testing.T) {
	now := time.Now()
	cases := []struct {
		offset   time.Duration
		expected string
	}{{14 * 24 * time.Hour, "green"}, {5 * 24 * time.Hour, "yellow"}, {2 * 24 * time.Hour, "red"}, {12 * time.Hour, "critical"}, {-2 * time.Hour, "critical"}}
	for _, tc := range cases {
		due := now.Add(tc.offset)
		if got := urgency(Task{DueAt: &due}); got != tc.expected {
			t.Errorf("%v: got %s, want %s", tc.offset, got, tc.expected)
		}
	}
	due := now.Add(12 * time.Hour)
	if got := urgency(Task{DueAt: &due, UrgencyOverride: "green"}); got != "green" {
		t.Errorf("manual override ignored: %s", got)
	}
	if got := urgency(Task{DueAt: &due, Status: "done"}); got != "green" {
		t.Errorf("completed task should not blink: %s", got)
	}
}
func uintString(x uint) string { return strconv.FormatUint(uint64(x), 10) }
func TestEndToEndReview(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires TEST_DATABASE_URL with disposable PostgreSQL")
	}
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrator().DropTable(&Review{}, &Task{}, &Membership{}, &Project{}, &Session{}, &User{}); err != nil {
		t.Fatal(err)
	}
	if err := migrate(db); err != nil {
		t.Fatal(err)
	}
	a := &App{db: db, secret: []byte("0123456789abcdefghij0123456789abcdefghij"), statsCache: newStatsCache(10 * time.Second)}
	router := a.routes()
	send := func(method, path, token string, payload any) (int, map[string]any) {
		t.Helper()
		var body []byte
		if payload != nil {
			body, _ = json.Marshal(payload)
		}
		req := httptest.NewRequest(method, path, bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		var resp map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		return w.Code, resp
	}
	check := func(got, want int, body map[string]any) {
		t.Helper()
		if got != want {
			t.Fatalf("status %d != %d response: %+v", got, want, body)
		}
	}
	code, resp := send(http.MethodPost, "/api/auth/register", "", map[string]any{"name": "Alice", "email": "alice@example.com", "password": "long_password_2026"})
	check(code, 200, resp)
	aToken := resp["token"].(string)
	code, resp = send(http.MethodPost, "/api/auth/register", "", map[string]any{"name": "Bob", "email": "bob@example.com", "password": "another_password_2026"})
	check(code, 200, resp)
	bToken := resp["token"].(string)
	bID := uint(resp["user"].(map[string]any)["id"].(float64))
	code, resp = send(http.MethodPost, "/api/projects", aToken, map[string]any{"name": "Demo project", "description": "Demo"})
	check(code, 201, resp)
	pID := uint(resp["project"].(map[string]any)["id"].(float64))
	path := "/api/projects/" + uintString(pID)
	code, resp = send(http.MethodGet, path, bToken, nil)
	check(code, 403, resp)
	code, resp = send(http.MethodPost, path+"/invitations", aToken, map[string]any{"email": "bob@example.com"})
	check(code, 201, resp)
	code, resp = send(http.MethodPost, path+"/tasks", aToken, map[string]any{"title": "Build homepage", "description": "Create a page", "priority": "high", "urgency_override": "auto", "assignee_id": bID})
	check(code, 201, resp)
	taskID := uint(resp["task"].(map[string]any)["id"].(float64))
	tp := path + "/tasks/" + uintString(taskID)
	code, resp = send(http.MethodPatch, tp, bToken, map[string]any{"status": "done"})
	check(code, 400, resp)
	code, resp = send(http.MethodPatch, tp, bToken, map[string]any{"status": "in_progress"})
	check(code, 200, resp)
	code, resp = send(http.MethodPost, tp+"/submit", bToken, map[string]any{"description": "Done", "evidence_url": "https://github.com"})
	check(code, 200, resp)
	code, resp = send(http.MethodPost, tp+"/review", bToken, map[string]any{"decision": "approved"})
	check(code, 403, resp)
	code, resp = send(http.MethodPost, tp+"/review", aToken, map[string]any{"decision": "approved", "comment": "OK"})
	check(code, 200, resp)
	code, resp = send(http.MethodGet, path+"/tasks", aToken, nil)
	check(code, 200, resp)
	task := resp["tasks"].([]any)[0].(map[string]any)
	if task["status"] != "done" || task["completed_at"] == nil {
		t.Fatalf("task was not completed: %+v", task)
	}
	code, resp = send(http.MethodGet, "/api/admin/users", aToken, nil)
	check(code, 403, resp)
	code, resp = send(http.MethodPost, "/api/auth/logout", bToken, nil)
	check(code, 200, resp)
	code, resp = send(http.MethodGet, "/api/dashboard", bToken, nil)
	check(code, 401, resp)
}