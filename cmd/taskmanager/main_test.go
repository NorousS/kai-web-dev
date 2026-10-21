package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
	"taskmanager/internal/model"
	"taskmanager/internal/web"
	"taskmanager/migrations"
)

func TestCLIHelperProcess(t *testing.T) {
	if os.Getenv("TASKMANAGER_CLI_HELPER") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			os.Args = append([]string{"taskmanager"}, os.Args[i+1:]...)
			if err := run(); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			os.Exit(0)
		}
	}
	os.Exit(2)
}

func cliDatabase(t *testing.T) (*model.Store, string) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("TEST_DATABASE_URL must point to a PostgreSQL database ending in _test")
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
		base.Close()
		t.Fatal(err)
	}
	schema := "cli_test_" + hex.EncodeToString(random[:])
	ident := pgx.Identifier{schema}.Sanitize()
	if _, err = base.Exec(ctx, "CREATE SCHEMA "+ident); err != nil {
		base.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, err := base.Exec(context.Background(), "DROP SCHEMA "+ident+" CASCADE")
		base.Close()
		if err != nil {
			t.Error(err)
		}
	})
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	scopedDSN := parsed.String()
	pool, err := pgxpool.New(ctx, scopedDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	contents, err := migrations.Files.ReadFile("001_initial.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := strings.Split(string(contents), "-- +goose Down")[0]
	if _, err = pool.Exec(ctx, up); err != nil {
		t.Fatal(err)
	}
	return model.New(pool), scopedDSN
}

func runAdminCLI(t *testing.T, dsn, email, password string) (string, error) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, "-test.run=^TestCLIHelperProcess$", "--", "create-admin", "--username", "Администратор", "--email", email)
	cmd.Env = append(os.Environ(), "TASKMANAGER_CLI_HELPER=1", "DATABASE_URL="+dsn)
	cmd.Stdin = strings.NewReader(password + "\n")
	output, err := cmd.CombinedOutput()
	if strings.Contains(string(output), password) {
		t.Fatal("CLI exposed plaintext password")
	}
	return string(output), err
}

func TestCreateAdminCLIAndLogin(t *testing.T) {
	for _, email := range []string{"admin@example.com", strings.Repeat("я", 45) + "@example.com"} {
		t.Run(email, func(t *testing.T) {
			store, dsn := cliDatabase(t)
			const password = "пароль123"
			output, err := runAdminCLI(t, dsn, email, password)
			if err != nil {
				t.Fatalf("create-admin: %v, output: %s", err, output)
			}
			u, err := store.UserByEmail(context.Background(), email)
			if err != nil {
				t.Fatal(err)
			}
			if u.Role != "admin" || u.Password == password || bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(password)) != nil {
				t.Fatal("CLI did not store an admin with a valid password hash")
			}
			server := httptest.NewServer(web.New(store, false))
			defer server.Close()
			jar, _ := cookiejar.New(nil)
			client := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
			res, err := client.Get(server.URL + "/login")
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(res.Body)
			res.Body.Close()
			if err != nil {
				t.Fatal(err)
			}
			csrf := regexp.MustCompile(`name="_csrf"[^>]*value="([^"]+)"`).FindSubmatch(body)
			if len(csrf) != 2 {
				t.Fatal("login form has no CSRF token")
			}
			res, err = client.PostForm(server.URL+"/login", url.Values{"email": {email}, "password": {password}, "_csrf": {string(csrf[1])}})
			if err != nil {
				t.Fatal(err)
			}
			res.Body.Close()
			if res.StatusCode != http.StatusSeeOther {
				t.Fatalf("login status: %d", res.StatusCode)
			}
			var sessionCookie *http.Cookie
			for _, cookie := range res.Cookies() {
				if cookie.Name == "session" {
					sessionCookie = cookie
				}
			}
			if sessionCookie == nil || sessionCookie.Value == "" || !sessionCookie.HttpOnly || sessionCookie.MaxAge != 86400 || sessionCookie.SameSite != http.SameSiteLaxMode {
				t.Fatalf("login session cookie missing or invalid: %v", sessionCookie)
			}
			var sessionCount int
			if err = store.Pool.QueryRow(context.Background(), "SELECT count(*) FROM sessions WHERE user_id=$1", u.ID).Scan(&sessionCount); err != nil || sessionCount != 1 {
				t.Fatalf("session count %d, error %v", sessionCount, err)
			}
			res, err = client.Get(server.URL + "/admin/users")
			if err != nil {
				t.Fatal(err)
			}
			res.Body.Close()
			if res.StatusCode != http.StatusOK {
				t.Fatalf("authenticated admin page: %d", res.StatusCode)
			}
			if output, err = runAdminCLI(t, dsn, strings.ToUpper(email), password); err == nil {
				t.Fatalf("duplicate email accepted: %s", output)
			}
			users, err := store.Users(context.Background())
			if err != nil || len(users) != 1 {
				t.Fatalf("duplicate changed users: %v %v", users, err)
			}
		})
	}
}

func TestCreateAdminCLIRejectsShortPassword(t *testing.T) {
	store, dsn := cliDatabase(t)
	if output, err := runAdminCLI(t, dsn, "short@example.com", "short"); err == nil {
		t.Fatalf("short password accepted: %s", output)
	}
	if _, err := store.UserByEmail(context.Background(), "short@example.com"); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("invalid admin persisted: %v", err)
	}
}
