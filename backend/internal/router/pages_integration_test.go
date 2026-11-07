package router

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/NorousS/kai-web-dev/backend/internal/migration"
	"github.com/NorousS/kai-web-dev/backend/internal/model"
	"github.com/NorousS/kai-web-dev/backend/internal/repository"
	"github.com/NorousS/kai-web-dev/backend/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestServerRenderedFormsWithPostgres(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires disposable PostgreSQL test database")
	}
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	// This integration test uses a disposable PostgreSQL database, never production.
	if err = db.Migrator().DropTable("schema_migrations", &model.Review{}, &model.Task{}, &model.Membership{}, &model.Project{}, &model.Session{}, &model.User{}); err != nil {
		t.Fatal(err)
	}
	if err = migration.Apply(db); err != nil {
		t.Fatal(err)
	}
	app := New(service.New(repository.New(db), "0123456789abcdefghij0123456789abcdefghij"), db, false)
	request := func(method, path string, values url.Values, cookies ...*http.Cookie) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, nil)
		if method == http.MethodPost {
			req = httptest.NewRequest(method, path, strings.NewReader(values.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		for _, cookie := range cookies {
			req.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		return w
	}
	login := request("GET", "/register", nil)
	if login.Code != 200 || !strings.Contains(login.Body.String(), "<html") {
		t.Fatalf("register not server-rendered: %d %s", login.Code, login.Body.String())
	}
	var csrf *http.Cookie
	for _, cookie := range login.Result().Cookies() {
		if cookie.Name == "tt_csrf" {
			csrf = cookie
		}
	}
	if csrf == nil {
		t.Fatal("CSRF cookie not set")
	}
	values := url.Values{"_csrf": {csrf.Value}, "name": {"Demo Student"}, "email": {"demo.student@example.org"}, "password": {"strong_password_2026"}}
	register := request("POST", "/register", values, csrf)
	if register.Code != 303 {
		t.Fatalf("register response %d %s", register.Code, register.Body.String())
	}
	var session *http.Cookie
	for _, cookie := range register.Result().Cookies() {
		if cookie.Name == "tt_session" {
			session = cookie
		}
	}
	if session == nil || !session.HttpOnly {
		t.Fatal("JWT HttpOnly cookie not issued")
	}
	home := request("GET", "/", nil, csrf, session)
	if home.Code != 200 || !strings.Contains(home.Body.String(), "Статистика") {
		t.Fatalf("dashboard: %d %s", home.Code, home.Body.String())
	}
	blocked := request("POST", "/projects", url.Values{"name": {"Bad project"}}, session)
	if blocked.Code != 403 {
		t.Fatalf("missing CSRF accepted: %d", blocked.Code)
	}
	create := request("POST", "/projects", url.Values{"_csrf": {csrf.Value}, "name": {"Study project"}, "description": {"MVC test"}}, csrf, session)
	if create.Code != 303 {
		t.Fatalf("project not created: %d %s", create.Code, create.Body.String())
	}
	projectURL := create.Header().Get("Location")
	project := request("GET", projectURL, nil, csrf, session)
	if project.Code != 200 || !strings.Contains(project.Body.String(), "Study project") {
		t.Fatalf("project page: %d %s", project.Code, project.Body.String())
	}
	taskForm := request("GET", projectURL+"/tasks/new", nil, csrf, session)
	if taskForm.Code != 200 || !strings.Contains(taskForm.Body.String(), "Базовый приоритет") {
		t.Fatalf("task form: %d %s", taskForm.Code, taskForm.Body.String())
	}
	task := request("POST", projectURL+"/tasks", url.Values{"_csrf": {csrf.Value}, "title": {"Write documentation"}, "description": {"SSR task"}, "priority": {"low"}, "urgency_override": {"auto"}}, csrf, session)
	if task.Code != 303 {
		t.Fatalf("task not created: %d %s", task.Code, task.Body.String())
	}
	project = request("GET", projectURL, nil, csrf, session)
	if project.Code != 200 || !strings.Contains(project.Body.String(), "Write documentation") {
		t.Fatalf("task not rendered in HTML: %d %s", project.Code, project.Body.String())
	}
}