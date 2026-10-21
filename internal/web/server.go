package web

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"taskmanager/internal/model"
)

//go:embed templates.html style.css bootstrap.min.css
var assets embed.FS

type server struct {
	store     *model.Store
	secure    bool
	key       []byte
	templates *template.Template
	location  *time.Location
}

func New(store *model.Store, secure bool) *gin.Engine {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		panic(err)
	}
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		panic(err)
	}
	s := &server{store: store, secure: secure, key: key, location: loc}
	s.templates = template.Must(template.New("pages").Funcs(template.FuncMap{
		"status": statusLabel, "priority": priorityLabel, "date": func(t time.Time) string { return t.In(loc).Format("02.01.2006 15:04") },
		"due": func(t *time.Time) string {
			if t == nil {
				return "Не указан"
			}
			return t.In(loc).Format("02.01.2006 15:04")
		},
	}).ParseFS(assets, "templates.html"))
	r := gin.New()
	r.Use(gin.Recovery(), s.middleware)
	r.GET("/favicon.ico", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	r.GET("/static/style.css", func(c *gin.Context) { b, _ := assets.ReadFile("style.css"); c.Data(200, "text/css; charset=utf-8", b) })
	r.GET("/static/bootstrap.min.css", func(c *gin.Context) {
		b, _ := assets.ReadFile("bootstrap.min.css")
		c.Data(200, "text/css; charset=utf-8", b)
	})
	r.GET("/", func(c *gin.Context) { s.render(c, 200, "home", nil) })
	r.GET("/register", func(c *gin.Context) { s.render(c, 200, "register", nil) })
	r.POST("/register", s.register)
	r.GET("/login", func(c *gin.Context) { s.render(c, 200, "login", nil) })
	r.POST("/login", s.login)
	r.POST("/logout", s.logout)
	p := r.Group("", s.requireUser)
	p.GET("/tasks", s.tasks)
	p.GET("/tasks/new", func(c *gin.Context) {
		s.render(c, 200, "taskform", gin.H{"Action": "/tasks/new", "Form": map[string]string{"status": "new", "priority": "medium"}})
	})
	p.POST("/tasks", s.createTask)
	p.GET("/tasks/:id/delete", func(c *gin.Context) {
		t, ok := s.readTask(c, false)
		if ok {
			s.showTask(c, 200, t, nil)
		}
	})
	p.POST("/tasks/new", s.createTask)
	p.GET("/tasks/:id", s.task)
	p.GET("/tasks/:id/edit", s.editPage)
	p.POST("/tasks/:id/edit", s.editTask)
	p.POST("/tasks/:id/delete", s.deleteTask)
	p.POST("/tasks/:id/comments", s.comment)
	p.GET("/profile", s.profilePage)
	p.POST("/profile", s.profile)
	a := p.Group("/admin", s.requireAdmin)
	a.GET("/users", s.users)
	a.GET("/tasks", s.adminTasks)
	a.POST("/tasks/:id/delete", s.adminDelete)
	a.POST("/users/:id/block", func(c *gin.Context) { s.block(c, true) })
	a.POST("/users/:id/unblock", func(c *gin.Context) { s.block(c, false) })
	r.NoRoute(func(c *gin.Context) { s.fail(c, 404, "Страница не найдена") })
	return r
}
func statusLabel(v string) string {
	switch v {
	case "new":
		return "Новая"
	case "in_progress":
		return "В работе"
	case "completed":
		return "Выполнена"
	}
	return v
}
func priorityLabel(v string) string {
	switch v {
	case "low":
		return "Низкий"
	case "medium":
		return "Средний"
	case "high":
		return "Высокий"
	}
	return v
}
func randomToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
func tokenHash(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}
func (s *server) cookie(c *gin.Context, name, value string, age int) {
	http.SetCookie(c.Writer, &http.Cookie{Name: name, Value: value, Path: "/", MaxAge: age, HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteLaxMode})
}
func (s *server) signature(token, session string) string {
	h := hmac.New(sha256.New, s.key)
	h.Write([]byte(token))
	h.Write([]byte{0})
	h.Write([]byte(session))
	return hex.EncodeToString(h.Sum(nil))
}
func (s *server) middleware(c *gin.Context) {
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cache-Control", "no-store")
	session, _ := c.Cookie("session")
	// Bind forms to the effective session, after discarding expired/revoked cookies.
	if session != "" {
		user, err := s.store.SessionUser(c.Request.Context(), tokenHash(session))
		if err == nil && !user.IsBlocked {
			c.Set("user", user)
		} else if err != nil && !errors.Is(err, model.ErrNotFound) {
			s.dbError(c, err)
			c.Abort()
			return
		} else {
			s.cookie(c, "session", "", -1)
			session = ""
		}
	}
	csrf, _ := c.Cookie("csrf")
	parts := strings.Split(csrf, ".")
	if len(parts) != 2 || !hmac.Equal([]byte(parts[1]), []byte(s.signature(parts[0], session))) {
		csrf = randomToken()
		csrf += "." + s.signature(csrf, session)
		s.cookie(c, "csrf", csrf, 86400)
	}
	c.Set("csrf", csrf)
	if c.Request.Method == http.MethodPost {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
		if err := c.Request.ParseForm(); err != nil {
			s.fail(c, 413, "Форма слишком большая или повреждена")
			c.Abort()
			return
		}
		if !hmac.Equal([]byte(c.PostForm("_csrf")), []byte(csrf)) {
			s.fail(c, 403, "Не удалось проверить форму. Обновите страницу.")
			c.Abort()
			return
		}
	}
	c.Next()
}
func current(c *gin.Context) model.User { v, _ := c.Get("user"); u, _ := v.(model.User); return u }
func (s *server) requireUser(c *gin.Context) {
	if current(c).ID == 0 {
		c.Redirect(303, "/login")
		c.Abort()
		return
	}
	c.Next()
}
func (s *server) requireAdmin(c *gin.Context) {
	if current(c).Role != "admin" {
		s.fail(c, 403, "Доступ запрещён")
		c.Abort()
		return
	}
	c.Next()
}
func (s *server) render(c *gin.Context, status int, page string, data gin.H) {
	if data == nil {
		data = gin.H{}
	}
	data["Page"] = page
	data["User"] = current(c)
	data["CSRF"], _ = c.Get("csrf")
	if _, ok := data["Form"]; !ok {
		data["Form"] = map[string]string{}
	}
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Status(status)
	if err := s.templates.ExecuteTemplate(c.Writer, "layout", data); err != nil {
		slog.Error("Ошибка вывода страницы", "page", page, "error", err)
		_ = c.Error(err)
	}
}
func (s *server) fail(c *gin.Context, status int, message string) {
	s.render(c, status, "error", gin.H{"Error": message})
}
func (s *server) dbError(c *gin.Context, err error) {
	if errors.Is(err, model.ErrNotFound) {
		s.fail(c, 404, "Не найдено")
	} else if errors.Is(err, model.ErrForbidden) {
		s.fail(c, 403, "Доступ запрещён")
	} else {
		slog.Error("Ошибка обращения к БД", "error", err)
		s.fail(c, 503, "База данных временно недоступна")
	}
}
func form(c *gin.Context) map[string]string {
	f := map[string]string{}
	for _, key := range []string{"username", "email", "title", "description", "status", "priority", "due_date", "text"} {
		f[key] = c.PostForm(key)
	}
	return f
}
func normalizeEmail(v string) string { return strings.ToLower(strings.TrimSpace(v)) }
func identity(f map[string]string) string {
	f["username"] = strings.TrimSpace(f["username"])
	f["email"] = normalizeEmail(f["email"])
	if n := utf8.RuneCountInString(f["username"]); n < 3 || n > 50 {
		return "Имя должно содержать от 3 до 50 символов"
	}
	a, err := mail.ParseAddress(f["email"])
	if err != nil || a.Address != f["email"] || utf8.RuneCountInString(f["email"]) > 100 {
		return "Укажите корректный email, до 100 символов"
	}
	return ""
}
func (s *server) register(c *gin.Context) {
	f := form(c)
	msg := identity(f)
	password := c.PostForm("password")
	if msg == "" && (utf8.RuneCountInString(password) < 8 || len(password) > 72) {
		msg = "Пароль должен содержать от 8 символов и не более 72 байт"
	}
	if msg != "" {
		s.render(c, 422, "register", gin.H{"Form": f, "Error": msg})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		s.fail(c, 500, "Не удалось обработать пароль")
		return
	}
	_, err = s.store.CreateUser(c.Request.Context(), f["username"], f["email"], string(hash), "user")
	if errors.Is(err, model.ErrConflict) {
		s.render(c, 422, "register", gin.H{"Form": f, "Error": "Этот email уже зарегистрирован"})
		return
	}
	if err != nil {
		s.dbError(c, err)
		return
	}
	c.Redirect(303, "/login")
}
func (s *server) login(c *gin.Context) {
	f := form(c)
	f["email"] = normalizeEmail(f["email"])
	u, err := s.store.UserByEmail(c.Request.Context(), f["email"])
	if err != nil && !errors.Is(err, model.ErrNotFound) {
		s.dbError(c, err)
		return
	}
	if err != nil || u.IsBlocked || bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(c.PostForm("password"))) != nil {
		s.render(c, 422, "login", gin.H{"Form": f, "Error": "Неверный email или пароль либо учётная запись заблокирована"})
		return
	}
	token := randomToken()
	if err = s.store.CreateSession(c.Request.Context(), u.ID, tokenHash(token), time.Now().Add(24*time.Hour)); err != nil {
		s.dbError(c, err)
		return
	}
	s.cookie(c, "session", token, 86400)
	c.Redirect(303, "/tasks")
}
func (s *server) logout(c *gin.Context) {
	token, _ := c.Cookie("session")
	if token != "" {
		if err := s.store.DeleteSession(c.Request.Context(), tokenHash(token)); err != nil {
			s.dbError(c, err)
			return
		}
	}
	s.cookie(c, "session", "", -1)
	c.Redirect(303, "/login")
}
func (s *server) tasks(c *gin.Context)      { s.list(c, false) }
func (s *server) adminTasks(c *gin.Context) { s.list(c, true) }
func (s *server) list(c *gin.Context, all bool) {
	status := c.Query("status")
	if status != "" && status != "new" && status != "in_progress" && status != "completed" {
		s.fail(c, 422, "Неизвестный статус")
		return
	}
	items, err := s.store.Tasks(c.Request.Context(), current(c).ID, all, c.Query("q"), status)
	if err != nil {
		s.dbError(c, err)
		return
	}
	s.render(c, 200, "tasks", gin.H{"Tasks": items, "All": all, "Query": c.Query("q"), "Status": status})
}
func (s *server) readTask(c *gin.Context, all bool) (model.Task, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id < 1 {
		s.fail(c, 404, "Задача не найдена")
		return model.Task{}, false
	}
	t, err := s.store.TaskByID(c.Request.Context(), id, current(c).ID, all)
	if err != nil {
		s.dbError(c, err)
		return t, false
	}
	return t, true
}
func (s *server) showTask(c *gin.Context, status int, t model.Task, extra gin.H) {
	comments, err := s.store.Comments(c.Request.Context(), t.ID)
	if err != nil {
		s.dbError(c, err)
		return
	}
	if extra == nil {
		extra = gin.H{}
	}
	extra["Task"] = t
	extra["Comments"] = comments
	extra["Owner"] = t.UserID == current(c).ID
	s.render(c, status, "task", extra)
}
func (s *server) task(c *gin.Context) {
	t, ok := s.readTask(c, current(c).Role == "admin")
	if ok {
		s.showTask(c, 200, t, nil)
	}
}
func (s *server) taskInput(c *gin.Context, create bool) (model.Task, map[string]string, string) {
	f := form(c)
	f["title"] = strings.TrimSpace(f["title"])
	t := model.Task{Title: f["title"], Description: f["description"], Status: f["status"], Priority: f["priority"]}
	if create {
		if t.Status == "" {
			t.Status = "new"
			f["status"] = "new"
		}
		if t.Priority == "" {
			t.Priority = "medium"
			f["priority"] = "medium"
		}
	}
	if t.Title == "" || utf8.RuneCountInString(t.Title) > 150 {
		return t, f, "Название обязательно, не более 150 символов"
	}
	if t.Status != "new" && t.Status != "in_progress" && t.Status != "completed" {
		return t, f, "Выберите допустимый статус"
	}
	if t.Priority != "low" && t.Priority != "medium" && t.Priority != "high" {
		return t, f, "Выберите допустимый приоритет"
	}
	if f["due_date"] != "" {
		date, err := time.ParseInLocation("2006-01-02T15:04", f["due_date"], s.location)
		if err != nil {
			return t, f, "Укажите корректный срок выполнения"
		}
		t.DueDate = &date
	}
	return t, f, ""
}
func (s *server) createTask(c *gin.Context) {
	t, f, msg := s.taskInput(c, true)
	if msg != "" {
		s.render(c, 422, "taskform", gin.H{"Form": f, "Action": "/tasks/new", "Error": msg})
		return
	}
	id, err := s.store.CreateTask(c.Request.Context(), current(c).ID, t)
	if err != nil {
		s.dbError(c, err)
		return
	}
	c.Redirect(303, "/tasks/"+strconv.Itoa(id))
}
func (s *server) editPage(c *gin.Context) {
	t, ok := s.readTask(c, false)
	if !ok {
		return
	}
	f := map[string]string{"title": t.Title, "description": t.Description, "status": t.Status, "priority": t.Priority}
	if t.DueDate != nil {
		f["due_date"] = t.DueDate.In(s.location).Format("2006-01-02T15:04")
	}
	s.render(c, 200, "taskform", gin.H{"Form": f, "Action": c.Request.URL.Path, "Editing": true})
}
func (s *server) editTask(c *gin.Context) {
	old, ok := s.readTask(c, false)
	if !ok {
		return
	}
	t, f, msg := s.taskInput(c, false)
	if msg != "" {
		s.render(c, 422, "taskform", gin.H{"Form": f, "Action": c.Request.URL.Path, "Editing": true, "Error": msg})
		return
	}
	if err := s.store.UpdateTask(c.Request.Context(), old.ID, current(c).ID, t); err != nil {
		s.dbError(c, err)
		return
	}
	c.Redirect(303, "/tasks/"+strconv.Itoa(old.ID))
}
func (s *server) deleteTask(c *gin.Context)  { s.remove(c, false) }
func (s *server) adminDelete(c *gin.Context) { s.remove(c, true) }
func (s *server) remove(c *gin.Context, all bool) {
	t, ok := s.readTask(c, all)
	if !ok {
		return
	}
	if c.PostForm("confirm") != "yes" {
		s.showTask(c, 422, t, gin.H{"Error": "Подтвердите удаление задачи"})
		return
	}
	if err := s.store.DeleteTask(c.Request.Context(), t.ID, current(c).ID, all); err != nil {
		s.dbError(c, err)
		return
	}
	target := "/tasks"
	if all {
		target = "/admin/tasks"
	}
	c.Redirect(303, target)
}
func (s *server) comment(c *gin.Context) {
	t, ok := s.readTask(c, false)
	if !ok {
		return
	}
	value := strings.TrimSpace(c.PostForm("text"))
	if value == "" {
		s.showTask(c, 422, t, gin.H{"Error": "Комментарий не должен быть пустым", "Form": form(c)})
		return
	}
	if err := s.store.AddComment(c.Request.Context(), t.ID, current(c).ID, value); err != nil {
		s.dbError(c, err)
		return
	}
	c.Redirect(303, "/tasks/"+strconv.Itoa(t.ID))
}
func (s *server) profilePage(c *gin.Context) {
	u := current(c)
	s.render(c, 200, "profile", gin.H{"Form": map[string]string{"username": u.Username, "email": u.Email}})
}
func (s *server) profile(c *gin.Context) {
	f := form(c)
	msg := identity(f)
	if msg == "" {
		err := s.store.UpdateProfile(c.Request.Context(), current(c).ID, f["username"], f["email"])
		if errors.Is(err, model.ErrConflict) {
			msg = "Этот email уже зарегистрирован"
		} else if err != nil {
			s.dbError(c, err)
			return
		}
	}
	if msg != "" {
		s.render(c, 422, "profile", gin.H{"Form": f, "Error": msg})
		return
	}
	c.Redirect(303, "/profile")
}
func (s *server) users(c *gin.Context) {
	users, err := s.store.Users(c.Request.Context())
	if err != nil {
		s.dbError(c, err)
		return
	}
	s.render(c, 200, "users", gin.H{"Users": users})
}
func (s *server) block(c *gin.Context, blocked bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id < 1 {
		s.fail(c, 404, "Пользователь не найден")
		return
	}
	if id == current(c).ID {
		s.fail(c, 403, "Нельзя блокировать себя")
		return
	}
	if err = s.store.SetBlocked(c.Request.Context(), id, blocked); err != nil {
		s.dbError(c, err)
		return
	}
	c.Redirect(303, "/admin/users")
}
