package router

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"

	"github.com/NorousS/kai-web-dev/backend/internal/migration"
	"github.com/NorousS/kai-web-dev/backend/internal/model"
	"github.com/NorousS/kai-web-dev/backend/internal/repository"
	"github.com/NorousS/kai-web-dev/backend/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestReviewWorkflowHTTP(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires disposable TEST_DATABASE_URL PostgreSQL")
	}
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Migrator().DropTable("schema_migrations", &model.Review{}, &model.Task{}, &model.Membership{}, &model.Project{}, &model.Session{}, &model.User{}); err != nil {
		t.Fatal(err)
	}
	if err = migration.Apply(db); err != nil {
		t.Fatal(err)
	}
	app := New(service.New(repository.New(db), "0123456789abcdefghij0123456789abcdefghij"), db, false)
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
		app.ServeHTTP(w, req)
		var resp map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		return w.Code, resp
	}
	check := func(got, want int, body map[string]any) {
		t.Helper()
		if got != want {
			t.Fatalf("HTTP %d != %d: %#v", got, want, body)
		}
	}
	code, resp := send("POST", "/api/auth/register", "", map[string]any{"name": "Alice", "email": "alice@example.com", "password": "long_password_2026"})
	check(code, 200, resp)
	owner := resp["token"].(string)
	code, resp = send("POST", "/api/auth/register", "", map[string]any{"name": "Bob", "email": "bob@example.com", "password": "another_password_2026"})
	check(code, 200, resp)
	worker := resp["token"].(string)
	workerID := uint(resp["user"].(map[string]any)["id"].(float64))
	code, resp = send("POST", "/api/projects", owner, map[string]any{"name": "Demo project", "description": "Demo"})
	check(code, 201, resp)
	pid := uint(resp["project"].(map[string]any)["id"].(float64))
	base := "/api/projects/" + strconv.FormatUint(uint64(pid), 10)
	code, resp = send("GET", base, worker, nil)
	check(code, 403, resp)
	code, resp = send("POST", base+"/invitations", owner, map[string]any{"email": "bob@example.com"})
	check(code, 201, resp)
	code, resp = send("POST", base+"/tasks", owner, map[string]any{"title": "Build homepage", "description": "Create a page", "priority": "high", "urgency_override": "auto", "assignee_id": workerID})
	check(code, 201, resp)
	taskID := uint(resp["task"].(map[string]any)["id"].(float64))
	path := base + "/tasks/" + strconv.FormatUint(uint64(taskID), 10)
	code, resp = send("PATCH", path, worker, map[string]any{"status": "done"})
	check(code, 400, resp)
	code, resp = send("PATCH", path, worker, map[string]any{"status": "in_progress"})
	check(code, 200, resp)
	code, resp = send("POST", path+"/submit", worker, map[string]any{})
	check(code, 400, resp)
	code, resp = send("POST", path+"/submit", worker, map[string]any{"description": "Completed work", "evidence_url": "https://github.com"})
	check(code, 200, resp)
	code, resp = send("POST", path+"/review", worker, map[string]any{"decision": "approved"})
	check(code, 403, resp)
	code, resp = send("POST", path+"/review", owner, map[string]any{"decision": "approved", "comment": "OK"})
	check(code, 200, resp)
	code, resp = send("GET", base+"/tasks", owner, nil)
	check(code, 200, resp)
	task := resp["tasks"].([]any)[0].(map[string]any)
	if task["status"] != "done" || task["completed_at"] == nil {
		t.Fatalf("task not completed: %#v", task)
	}
	code, resp = send("GET", "/api/admin/users", owner, nil)
	check(code, 403, resp)
	code, resp = send("POST", "/api/auth/logout", worker, nil)
	check(code, 200, resp)
	code, resp = send("GET", "/api/dashboard", worker, nil)
	check(code, 401, resp)
}