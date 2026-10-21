package web_test

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
	"taskmanager/internal/model"
	"taskmanager/internal/web"
)

const testPassword = "пароль123"

var csrfField = regexp.MustCompile(`name="_csrf"[^>]*value="([^"]+)"`)

type fixture struct {
	t      *testing.T
	store  *model.Store
	server *httptest.Server
}
type browser struct {
	f      *fixture
	client *http.Client
}

func setup(t *testing.T) *fixture {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("TEST_DATABASE_URL must point to an isolated PostgreSQL database ending in _test")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(cfg.ConnConfig.Database, "_test") {
		t.Fatal("refusing non-test database")
	}
	ctx := context.Background()
	base, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	var random [12]byte
	if _, err = rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	schema := "http_test_" + hex.EncodeToString(random[:])
	ident := pgx.Identifier{schema}.Sanitize()
	if _, err = base.Exec(ctx, "CREATE SCHEMA "+ident); err != nil {
		base.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, e := base.Exec(context.Background(), "DROP SCHEMA "+ident+" CASCADE")
		base.Close()
		if e != nil {
			t.Error(e)
		}
	})
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	migration, err := os.ReadFile("../../migrations/001_initial.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := strings.Split(string(migration), "-- +goose Down")[0]
	if _, err = pool.Exec(ctx, up); err != nil {
		t.Fatal(err)
	}
	f := &fixture{t: t, store: model.New(pool)}
	f.server = httptest.NewServer(web.New(f.store, false))
	t.Cleanup(f.server.Close)
	return f
}
func (f *fixture) browser() *browser {
	jar, _ := cookiejar.New(nil)
	return &browser{f: f, client: &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}
func (b *browser) request(method, path string, values url.Values, want int) (string, *http.Response) {
	b.f.t.Helper()
	var body io.Reader
	if values != nil {
		body = strings.NewReader(values.Encode())
	}
	req, err := http.NewRequest(method, b.f.server.URL+path, body)
	if err != nil {
		b.f.t.Fatal(err)
	}
	if values != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	res, err := b.client.Do(req)
	if err != nil {
		b.f.t.Fatal(err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		b.f.t.Fatal(err)
	}
	if res.StatusCode != want {
		b.f.t.Fatalf("%s %s: status=%d want=%d body=%s", method, path, res.StatusCode, want, data)
	}
	return string(data), res
}
func (b *browser) get(path string, want int) string {
	body, _ := b.request("GET", path, nil, want)
	return body
}
func (b *browser) post(form, path string, values url.Values, want int) (string, *http.Response) {
	b.f.t.Helper()
	body := b.get(form, 200)
	match := csrfField.FindStringSubmatch(body)
	if len(match) != 2 {
		b.f.t.Fatalf("CSRF hidden input absent in %s: %s", form, body)
	}
	if values == nil {
		values = url.Values{}
	}
	values.Set("_csrf", match[1])
	return b.request("POST", path, values, want)
}
func (f *fixture) seed(email, role string) model.User {
	f.t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(testPassword), bcrypt.MinCost)
	if err != nil {
		f.t.Fatal(err)
	}
	u, err := f.store.CreateUser(context.Background(), "Пользователь", email, string(hash), role)
	if err != nil {
		f.t.Fatal(err)
	}
	return u
}
func (b *browser) login(email string) *http.Response {
	_, res := b.post("/login", "/login", url.Values{"email": {email}, "password": {testPassword}}, 303)
	if res.Header.Get("Location") != "/tasks" {
		b.f.t.Fatalf("login redirect %s", res.Header.Get("Location"))
	}
	return res
}
func assertContains(t *testing.T, body string, parts ...string) {
	t.Helper()
	for _, p := range parts {
		if !strings.Contains(body, p) {
			t.Fatalf("missing %q in %s", p, body)
		}
	}
}
func assertAbsent(t *testing.T, body string, parts ...string) {
	t.Helper()
	for _, p := range parts {
		if strings.Contains(body, p) {
			t.Fatalf("unexpected %q in %s", p, body)
		}
	}
}
func (b *browser) create(title, status string) string {
	_, res := b.post("/tasks/new", "/tasks", url.Values{"title": {title}, "description": {"Описание задачи"}, "status": {status}}, 303)
	path := res.Header.Get("Location")
	if !regexp.MustCompile(`^/tasks/\d+$`).MatchString(path) {
		b.f.t.Fatalf("create redirect %q", path)
	}
	return path
}

func TestRegistrationValidationAndLogin(t *testing.T) {
	f := setup(t)
	b := f.browser()
	for _, tc := range []struct{ name, email, password string }{
		{"Яя", "a@example.com", testPassword}, {strings.Repeat("Я", 51), "a@example.com", testPassword},
		{"Антон", "invalid", testPassword}, {"Антон", "a@example.com", "1234567"},
		{"Антон", "a@example.com", strings.Repeat("Я", 37)},
	} {
		body, _ := b.post("/register", "/register", url.Values{"username": {tc.name}, "email": {tc.email}, "password": {tc.password}}, 422)
		assertAbsent(t, body, `value="`+tc.password+`"`)
	}
	b.post("/register", "/register", url.Values{"username": {strings.Repeat("Я", 50)}, "email": {" USER@EXAMPLE.COM "}, "password": {testPassword}, "role": {"admin"}}, 303)
	b.post("/register", "/register", url.Values{"username": {"Другой"}, "email": {"user@example.com"}, "password": {testPassword}}, 422)
	body, _ := b.post("/login", "/login", url.Values{"email": {"user@example.com"}, "password": {"wrong-secret"}}, 422)
	assertAbsent(t, body, "wrong-secret")
	res := b.login("user@example.com")
	var session *http.Cookie
	for _, c := range res.Cookies() {
		if c.Name == "session" {
			session = c
		}
	}
	if session == nil || !session.HttpOnly || session.SameSite != http.SameSiteLaxMode || session.Secure {
		t.Fatal("session cookie must be HttpOnly, SameSite=Lax, HTTP compatible")
	}
	b.get("/admin/users", 403)
	replay := f.browser()
	u, _ := url.Parse(f.server.URL)
	replay.client.Jar.SetCookies(u, []*http.Cookie{{Name: "session", Value: session.Value, Path: "/"}})
	b.post("/tasks", "/logout", nil, 303)
	replay.get("/tasks", 303)
}

func TestTasksOwnershipValidationCommentsAndDelete(t *testing.T) {
	f := setup(t)
	f.seed("owner@example.com", "user")
	f.seed("other@example.com", "user")
	owner, other := f.browser(), f.browser()
	owner.login("owner@example.com")
	other.login("other@example.com")
	for _, v := range []url.Values{{"title": {" "}}, {"title": {strings.Repeat("Я", 151)}}, {"title": {"Bad"}, "status": {"invented"}}, {"title": {"Bad"}, "priority": {"urgent"}}, {"title": {"Bad"}, "due_date": {"yesterday"}}} {
		owner.post("/tasks/new", "/tasks", v, 422)
	}
	path := owner.create(strings.Repeat("Я", 150), "")
	assertContains(t, owner.get(path, 200), strings.Repeat("Я", 150), "Описание задачи", "Новая", "Средний")
	assertAbsent(t, other.get("/tasks", 200), strings.Repeat("Я", 150))
	other.get(path, 404)
	other.get(path+"/edit", 404)
	other.post("/tasks", path+"/edit", url.Values{"title": {"Stolen"}}, 404)
	other.post("/tasks", path+"/comments", url.Values{"text": {"Stolen"}}, 404)
	other.post("/tasks", path+"/delete", url.Values{"confirm": {"yes"}}, 404)
	owner.post(path+"/edit", path+"/edit", url.Values{"title": {"Изменённая"}, "description": {"Новое описание"}, "status": {"completed"}, "priority": {"high"}, "due_date": {"2020-01-02T03:04"}}, 303)
	assertContains(t, owner.get(path, 200), "Изменённая", "Новое описание", "Выполнена", "Высокий", "2020")
	owner.post(path, path+"/comments", url.Values{"text": {"   "}}, 422)
	owner.post(path, path+"/comments", url.Values{"text": {"<script>alert('x')</script>"}}, 303)
	body := owner.get(path, 200)
	assertContains(t, body, "&lt;script&gt;", "Пользователь")
	assertAbsent(t, body, "<script>alert")
	owner.get(path+"/delete", 200)
	owner.get(path, 200)
	owner.post(path+"/delete", path+"/delete", url.Values{"confirm": {"no"}}, 422)
	owner.get(path, 200)
	owner.post(path+"/delete", path+"/delete", url.Values{"confirm": {"yes"}}, 303)
	owner.get(path, 404)
	owner.post("/tasks", path+"/comments", url.Values{"text": {"После удаления"}}, 404)
}

func TestSearchAndFilterWorkTogether(t *testing.T) {
	f := setup(t)
	f.seed("user@example.com", "user")
	b := f.browser()
	b.login("user@example.com")
	b.create("Купить хлеб", "new")
	b.create("КУПИТЬ молоко", "completed")
	b.create("Позвонить другу", "completed")
	body := b.get("/tasks?q="+url.QueryEscape("купить")+"&status=completed", 200)
	assertContains(t, body, "КУПИТЬ молоко", "купить")
	assertAbsent(t, body, "Купить хлеб", "Позвонить другу")
	body = b.get("/tasks", 200)
	if strings.Index(body, "Позвонить другу") > strings.Index(body, "Купить хлеб") {
		t.Fatal("newest tasks must appear first")
	}
}

func TestProfileValidationAndRoleCannotChange(t *testing.T) {
	f := setup(t)
	f.seed("first@example.com", "user")
	f.seed("taken@example.com", "user")
	b := f.browser()
	b.login("first@example.com")
	b.post("/profile", "/profile", url.Values{"username": {"Антон"}, "email": {"TAKEN@example.com"}}, 422)
	b.post("/profile", "/profile", url.Values{"username": {"Я"}, "email": {"new@example.com"}}, 422)
	b.post("/profile", "/profile", url.Values{"username": {"Новое имя"}, "email": {" NEW@example.com "}, "role": {"admin"}}, 303)
	assertContains(t, b.get("/profile", 200), "Новое имя", "new@example.com")
	b.get("/admin/users", 403)
	b.post("/profile", "/logout", nil, 303)
	b.login("new@example.com")
}

func TestAdminBlocksSessionsAndDeletesOtherTasks(t *testing.T) {
	f := setup(t)
	user := f.seed("user@example.com", "user")
	admin := f.seed("admin@example.com", "admin")
	b, a := f.browser(), f.browser()
	b.login("user@example.com")
	a.login("admin@example.com")
	path := b.create("Задача пользователя", "")
	u, _ := url.Parse(f.server.URL)
	oldCookies := b.client.Jar.Cookies(u)
	assertContains(t, a.get("/admin/users", 200), "user@example.com")
	assertContains(t, a.get("/admin/tasks", 200), "Задача пользователя")
	a.get(path, 200)
	a.get(path+"/edit", 404)
	a.post("/admin/users", fmt.Sprintf("/admin/users/%d/block", user.ID), nil, 303)
	b.get("/tasks", 303)
	b.post("/login", "/login", url.Values{"email": {"user@example.com"}, "password": {testPassword}}, 422)
	a.post("/admin/users", fmt.Sprintf("/admin/users/%d/unblock", user.ID), nil, 303)
	replay := f.browser()
	replay.client.Jar.SetCookies(u, oldCookies)
	replay.get("/tasks", 303)
	b.login("user@example.com")
	a.post("/admin/users", fmt.Sprintf("/admin/users/%d/block", admin.ID), nil, 403)
	a.post(path, "/admin"+path+"/delete", url.Values{"confirm": {"yes"}}, 303)
	b.get(path, 404)
}

func TestCSRFAndInvalidSessions(t *testing.T) {
	f := setup(t)
	b := f.browser()
	b.get("/register", 200)
	for _, path := range []string{"/register", "/login", "/logout", "/tasks", "/profile"} {
		b.request("POST", path, url.Values{}, 403)
	}
	user := f.seed("user@example.com", "user")
	token := "expired-session-token"
	hash := sha256.Sum256([]byte(token))
	if err := f.store.CreateSession(context.Background(), user.ID, hex.EncodeToString(hash[:]), time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(f.server.URL)
	for _, value := range []string{token, "forged"} {
		b.client.Jar.SetCookies(u, []*http.Cookie{{Name: "session", Value: value, Path: "/"}})
		b.get("/tasks", 303)
	}
}

func TestDatabaseFailureReturnsUnavailable(t *testing.T) {
	f := setup(t)
	f.seed("user@example.com", "user")
	b := f.browser()
	b.login("user@example.com")
	f.store.Pool.Close()
	body := b.get("/tasks", 503)
	assertAbsent(t, body, "pool closed", "SELECT ", "postgres://", testPassword)
}

func TestRegistrationAcceptsMinimumNameAndPasswordBoundaries(t *testing.T) {
	f := setup(t)
	for i, password := range []string{"12345678", strings.Repeat("Я", 36)} {
		b := f.browser()
		email := fmt.Sprintf("boundary%d@example.com", i)
		b.post("/register", "/register", url.Values{"username": {"Имя"}, "email": {email}, "password": {password}}, 303)
		b.post("/login", "/login", url.Values{"email": {email}, "password": {password}}, 303)
		b.get("/tasks", 200)
	}
}
