package controller

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/NorousS/kai-web-dev/backend/internal/model"
	"github.com/NorousS/kai-web-dev/backend/internal/repository"
	"github.com/NorousS/kai-web-dev/backend/internal/service"
	"github.com/NorousS/kai-web-dev/backend/internal/view"
	"github.com/gin-gonic/gin"
)

// Pages is the HTML MVC controller. It calls the same services as the JSON API
// but responds with server-rendered HTML and 303 See Other after form POSTs.
type Pages struct {
	S            *service.Service
	V            *view.Renderer
	SecureCookie bool
}

func NewPages(s *service.Service, secure bool) (*Pages, error) {
	renderer, err := view.NewRenderer()
	if err != nil {
		return nil, err
	}
	return &Pages{S: s, V: renderer, SecureCookie: secure}, nil
}

func (h *Pages) Render(c *gin.Context, page string, status int, data gin.H) {
	if data == nil {
		data = gin.H{}
	}
	if _, ok := data["Title"]; !ok {
		data["Title"] = "Задачи"
	}
	if csrf, exists := c.Get("csrf_token"); exists {
		data["CSRF"] = csrf
	}
	if user, exists := c.Get("user"); exists {
		data["User"] = user
	}
	if err := h.V.Render(c.Writer, status, page, data); err != nil {
		log.Printf("HTML template render: %v", err)
		if !c.Writer.Written() {
			c.String(http.StatusInternalServerError, "Не удалось сформировать страницу")
		}
	}
}

func (h *Pages) Failed(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	message := "Ошибка сервера"
	var se *service.Error
	if errors.As(err, &se) {
		status, message = se.Status, se.Message
	}
	h.Render(c, "error", status, gin.H{"Title": "Ошибка", "Error": message})
}

// CSRF uses a cryptographically random, HttpOnly, same-site cookie and a hidden
// form token. Mutating HTML form routes require equality of both tokens.
func (h *Pages) CSRF() gin.HandlerFunc {
	return func(c *gin.Context) {
		cookie, err := c.Cookie("tt_csrf")
		validCookie := err == nil && len(cookie) == 64
		if validCookie {
			_, err = hex.DecodeString(cookie)
			validCookie = err == nil
		}
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			field := c.PostForm("_csrf")
			if !validCookie || subtle.ConstantTimeCompare([]byte(cookie), []byte(field)) != 1 {
				c.String(http.StatusForbidden, "Проверка формы не пройдена. Обновите страницу и повторите действие.")
				c.Abort()
				return
			}
		}
		if !validCookie {
			bytes := make([]byte, 32)
			if _, err = rand.Read(bytes); err != nil {
				c.AbortWithStatus(http.StatusInternalServerError)
				return
			}
			cookie = hex.EncodeToString(bytes)
			c.SetSameSite(http.SameSiteStrictMode)
			c.SetCookie("tt_csrf", cookie, 7*24*60*60, "/", "", h.SecureCookie, true)
		}
		c.Set("csrf_token", cookie)
		c.Next()
	}
}

func (h *Pages) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		raw, err := c.Cookie("tt_session")
		if err != nil || raw == "" {
			c.Redirect(http.StatusSeeOther, "/login")
			c.Abort()
			return
		}
		user, session, err := h.S.Authenticate(raw)
		if err != nil {
			c.Redirect(http.StatusSeeOther, "/login")
			c.Abort()
			return
		}
		c.Set("user", user)
		c.Set("session", session)
		c.Next()
	}
}

func currentPageUser(c *gin.Context) model.User { return c.MustGet("user").(model.User) }
func (h *Pages) SetLogin(c *gin.Context, user model.User) error {
	token, err := h.S.Issue(user)
	if err != nil {
		return err
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("tt_session", token, 7*24*60*60, "/", "", h.SecureCookie, true)
	return nil
}
func (h *Pages) LoginPage(c *gin.Context) {
	h.Render(c, "auth", 200, gin.H{"Title": "Вход", "Register": false})
}
func (h *Pages) RegisterPage(c *gin.Context) {
	h.Render(c, "auth", 200, gin.H{"Title": "Регистрация", "Register": true})
}

func (h *Pages) Login(c *gin.Context) {
	email := strings.TrimSpace(c.PostForm("email"))
	user, err := h.S.Login(email, c.PostForm("password"))
	if err == nil {
		err = h.SetLogin(c, user)
	}
	if err != nil {
		h.Render(c, "auth", 401, gin.H{"Title": "Вход", "Email": email, "Error": err.Error()})
		return
	}
	c.Redirect(http.StatusSeeOther, "/")
}
func (h *Pages) Register(c *gin.Context) {
	name := strings.TrimSpace(c.PostForm("name"))
	email := strings.TrimSpace(c.PostForm("email"))
	password := c.PostForm("password")
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email || len(email) > 255 {
		h.Render(c, "auth", 400, gin.H{"Title": "Регистрация", "Register": true, "Name": name, "Email": email, "Error": "Некорректный email"})
		return
	}
	user, err := h.S.Register(name, email, password)
	if err == nil {
		err = h.SetLogin(c, user)
	}
	if err != nil {
		h.Render(c, "auth", 400, gin.H{"Title": "Регистрация", "Register": true, "Name": name, "Email": email, "Error": err.Error()})
		return
	}
	c.Redirect(http.StatusSeeOther, "/")
}
func (h *Pages) Logout(c *gin.Context) {
	session := c.MustGet("session").(model.Session)
	if err := h.S.Logout(session.ID); err != nil {
		h.Failed(c, err)
		return
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("tt_session", "", -1, "/", "", h.SecureCookie, true)
	c.Redirect(http.StatusSeeOther, "/login")
}

func (h *Pages) Dashboard(c *gin.Context) {
	user := currentPageUser(c)
	stats, err := h.S.Dashboard(user)
	if err != nil {
		h.Failed(c, err)
		return
	}
	projects, err := h.S.Projects(user, "")
	if err != nil {
		h.Failed(c, err)
		return
	}
	h.Render(c, "dashboard", 200, gin.H{"Title": "Обзор", "Stats": stats, "Projects": projects})
}
func (h *Pages) Projects(c *gin.Context) {
	search := strings.TrimSpace(c.Query("q"))
	projects, err := h.S.Projects(currentPageUser(c), search)
	if err != nil {
		h.Failed(c, err)
		return
	}
	h.Render(c, "projects", 200, gin.H{"Title": "Проекты", "Projects": projects, "Search": search})
}
func validName(value string, max int) bool {
	n := utf8.RuneCountInString(strings.TrimSpace(value))
	return n >= 2 && n <= max
}
func (h *Pages) CreateProject(c *gin.Context) {
	name := strings.TrimSpace(c.PostForm("name"))
	if !validName(name, 120) || len(c.PostForm("description")) > 5000 {
		h.Failed(c, &service.Error{Status: 400, Message: "Название должно содержать 2–120 символов"})
		return
	}
	p, err := h.S.CreateProject(currentPageUser(c), name, c.PostForm("description"))
	if err != nil {
		h.Failed(c, err)
		return
	}
	c.Redirect(http.StatusSeeOther, projectURL(p.ID))
}
func pageID(c *gin.Context, key string) (uint, error) {
	n, e := strconv.ParseUint(c.Param(key), 10, 32)
	if e != nil || n == 0 {
		return 0, &service.Error{Status: 400, Message: "Некорректный идентификатор"}
	}
	return uint(n), nil
}
func projectURL(id uint) string { return fmt.Sprintf("/projects/%d", id) }
func taskURL(projectID, taskID uint) string {
	return fmt.Sprintf("/projects/%d#task-%d", projectID, taskID)
}
func (h *Pages) Project(c *gin.Context) {
	id, e := pageID(c, "projectID")
	if e != nil {
		h.Failed(c, e)
		return
	}
	user := currentPageUser(c)
	p, members, e := h.S.ProjectDetails(user, id)
	if e != nil {
		h.Failed(c, e)
		return
	}
	filter := repository.TaskFilter{Status: strings.TrimSpace(c.Query("status")), Query: strings.TrimSpace(c.Query("q"))}
	if filter.Status != "" && filter.Status != "todo" && filter.Status != "in_progress" && filter.Status != "in_review" && filter.Status != "done" {
		filter.Status = ""
	}
	tasks, e := h.S.Tasks(user, id, filter)
	if e != nil {
		h.Failed(c, e)
		return
	}
	canManage := user.Role == "admin" || p.MyRole == "owner" || p.MyRole == "manager"
	cards := make([]PageTask, 0, len(tasks))
	for _, task := range tasks {
		mine := task.AssigneeID != nil && *task.AssigneeID == user.ID
		cards = append(cards, PageTask{TaskView: task,
			CanStart:  mine && task.Status == "todo",
			CanSubmit: mine && (task.Status == "todo" || task.Status == "in_progress"),
			CanReview: canManage && task.Status == "in_review" && task.LatestReview != nil && task.LatestReview.SubmittedBy != user.ID,
			CanEdit:   canManage && (task.Status == "todo" || task.Status == "in_progress"),
		})
	}
	h.Render(c, "project", 200, gin.H{"Title": p.Name, "Project": p, "Members": members, "Tasks": cards, "Columns": groupProjectTasks(cards), "CanManage": canManage, "IsOwner": p.OwnerID == user.ID, "CanDelete": p.OwnerID == user.ID || p.CreatedBy == user.ID, "Status": filter.Status, "Search": filter.Query})
}

type PageTask struct {
	service.TaskView
	CanStart, CanSubmit, CanReview, CanEdit bool
}

// TaskColumn represents a single server-rendered Kanban lane.
type TaskColumn struct {
	Key   string
	Title string
	Tasks []PageTask
}

// groupProjectTasks keeps a fixed order, including empty lanes.
func groupProjectTasks(tasks []PageTask) []TaskColumn {
	columns := []TaskColumn{
		{Key: "todo", Title: "К выполнению"},
		{Key: "in_progress", Title: "В работе"},
		{Key: "in_review", Title: "На ревью"},
		{Key: "done", Title: "Завершено"},
	}
	for _, task := range tasks {
		for i := range columns {
			if columns[i].Key == task.Status {
				columns[i].Tasks = append(columns[i].Tasks, task)
				break
			}
		}
	}
	return columns
}

func (h *Pages) UpdateProject(c *gin.Context) {
	id, e := pageID(c, "projectID")
	if e != nil {
		h.Failed(c, e)
		return
	}
	name := strings.TrimSpace(c.PostForm("name"))
	if !validName(name, 120) || len(c.PostForm("description")) > 5000 {
		h.Failed(c, &service.Error{Status: 400, Message: "Некорректное имя или описание"})
		return
	}
	_, e = h.S.UpdateProject(currentPageUser(c), id, name, c.PostForm("description"))
	if e != nil {
		h.Failed(c, e)
		return
	}
	c.Redirect(303, projectURL(id))
}
func (h *Pages) DeleteProject(c *gin.Context) {
	id, e := pageID(c, "projectID")
	if e != nil {
		h.Failed(c, e)
		return
	}
	if c.PostForm("confirm") != "yes" {
		h.Failed(c, &service.Error{Status: 400, Message: "Подтвердите удаление проекта"})
		return
	}
	if e = h.S.DeleteProject(currentPageUser(c), id); e != nil {
		h.Failed(c, e)
		return
	}
	c.Redirect(303, "/projects")
}
func (h *Pages) Invite(c *gin.Context) {
	id, e := pageID(c, "projectID")
	if e != nil {
		h.Failed(c, e)
		return
	}
	if e = h.S.Invite(currentPageUser(c), id, c.PostForm("email")); e != nil {
		h.Failed(c, e)
		return
	}
	c.Redirect(303, projectURL(id))
}
func (h *Pages) ChangeMember(c *gin.Context) {
	id, e := pageID(c, "projectID")
	if e != nil {
		h.Failed(c, e)
		return
	}
	userID, e := pageID(c, "userID")
	if e != nil {
		h.Failed(c, e)
		return
	}
	if e = h.S.ChangeMember(currentPageUser(c), id, userID, c.PostForm("role")); e != nil {
		h.Failed(c, e)
		return
	}
	c.Redirect(303, projectURL(id))
}
func (h *Pages) RemoveMember(c *gin.Context) {
	id, e := pageID(c, "projectID")
	if e != nil {
		h.Failed(c, e)
		return
	}
	userID, e := pageID(c, "userID")
	if e != nil {
		h.Failed(c, e)
		return
	}
	if e = h.S.RemoveMember(currentPageUser(c), id, userID); e != nil {
		h.Failed(c, e)
		return
	}
	if userID == currentPageUser(c).ID {
		c.Redirect(303, "/projects")
		return
	}
	c.Redirect(303, projectURL(id))
}
func (h *Pages) TaskForm(c *gin.Context) {
	id, e := pageID(c, "projectID")
	if e != nil {
		h.Failed(c, e)
		return
	}
	user := currentPageUser(c)
	p, members, e := h.S.ProjectDetails(user, id)
	if e != nil {
		h.Failed(c, e)
		return
	}
	if !(user.Role == "admin" || p.MyRole == "owner" || p.MyRole == "manager") {
		h.Failed(c, &service.Error{Status: 403, Message: "Только управляющий может изменять задачи"})
		return
	}
	data := gin.H{"Title": "Новая задача", "Project": p, "Members": members, "Edit": false}
	if c.Param("taskID") != "" {
		taskID, err := pageID(c, "taskID")
		if err != nil {
			h.Failed(c, err)
			return
		}
		task, err := h.S.TaskDetails(user, id, taskID)
		if err != nil {
			h.Failed(c, err)
			return
		}
		if task.Status == "done" || task.Status == "in_review" {
			h.Failed(c, &service.Error{Status: 409, Message: "Эту задачу сейчас нельзя редактировать"})
			return
		}
		data["Edit"] = true
		data["Task"] = task
		data["Title"] = "Изменить задачу"
	}
	h.Render(c, "task_form", 200, data)
}
func parseTaskForm(c *gin.Context) (service.TaskInput, error) {
	var in service.TaskInput
	in.Title = strings.TrimSpace(c.PostForm("title"))
	in.Description = c.PostForm("description")
	in.Priority = c.PostForm("priority")
	in.UrgencyOverride = c.DefaultPostForm("urgency_override", "auto")
	in.Epic = strings.TrimSpace(c.PostForm("epic"))
	in.Tags = strings.TrimSpace(c.PostForm("tags"))
	if !validName(in.Title, 160) || len(in.Description) > 10000 || len(in.Epic) > 100 || len(in.Tags) > 400 {
		return in, &service.Error{Status: 400, Message: "Проверьте название, описание и теги"}
	}
	switch in.Priority {
	case "low", "medium", "high", "critical":
	default:
		return in, &service.Error{Status: 400, Message: "Некорректный приоритет"}
	}
	switch in.UrgencyOverride {
	case "auto", "green", "yellow", "red", "critical":
	default:
		return in, &service.Error{Status: 400, Message: "Некорректный индикатор"}
	}
	if raw := strings.TrimSpace(c.PostForm("assignee_id")); raw != "" {
		id, e := strconv.ParseUint(raw, 10, 32)
		if e != nil || id == 0 {
			return in, &service.Error{Status: 400, Message: "Некорректный исполнитель"}
		}
		v := uint(id)
		in.AssigneeID = &v
	}
	if raw := strings.TrimSpace(c.PostForm("due_at")); raw != "" {
		due, e := time.ParseInLocation("2006-01-02T15:04", raw, time.UTC)
		if e != nil {
			return in, &service.Error{Status: 400, Message: "Неверный формат дедлайна"}
		}
		in.DueAt = &due
	}
	return in, nil
}
func (h *Pages) CreateTask(c *gin.Context) {
	id, e := pageID(c, "projectID")
	if e != nil {
		h.Failed(c, e)
		return
	}
	in, e := parseTaskForm(c)
	if e != nil {
		h.Failed(c, e)
		return
	}
	task, e := h.S.CreateTask(currentPageUser(c), id, in)
	if e != nil {
		h.Failed(c, e)
		return
	}
	c.Redirect(303, taskURL(id, task.ID))
}
func (h *Pages) UpdateTask(c *gin.Context) {
	id, e := pageID(c, "projectID")
	if e != nil {
		h.Failed(c, e)
		return
	}
	taskID, e := pageID(c, "taskID")
	if e != nil {
		h.Failed(c, e)
		return
	}
	in, e := parseTaskForm(c)
	if e != nil {
		h.Failed(c, e)
		return
	}
	_, e = h.S.UpdateTask(currentPageUser(c), id, taskID, in, "")
	if e != nil {
		h.Failed(c, e)
		return
	}
	c.Redirect(303, taskURL(id, taskID))
}
func (h *Pages) StartTask(c *gin.Context) {
	id, e := pageID(c, "projectID")
	if e != nil {
		h.Failed(c, e)
		return
	}
	taskID, e := pageID(c, "taskID")
	if e != nil {
		h.Failed(c, e)
		return
	}
	if e = h.S.StartAssignedTask(currentPageUser(c), id, taskID); e != nil {
		h.Failed(c, e)
		return
	}
	c.Redirect(303, taskURL(id, taskID))
}
func (h *Pages) SubmitTask(c *gin.Context) {
	id, e := pageID(c, "projectID")
	if e != nil {
		h.Failed(c, e)
		return
	}
	taskID, e := pageID(c, "taskID")
	if e != nil {
		h.Failed(c, e)
		return
	}
	if e = h.S.SubmitTask(currentPageUser(c), id, taskID, c.PostForm("description"), c.PostForm("evidence_url")); e != nil {
		h.Failed(c, e)
		return
	}
	c.Redirect(303, taskURL(id, taskID))
}
func (h *Pages) ReviewTask(c *gin.Context) {
	id, e := pageID(c, "projectID")
	if e != nil {
		h.Failed(c, e)
		return
	}
	taskID, e := pageID(c, "taskID")
	if e != nil {
		h.Failed(c, e)
		return
	}
	decision := c.PostForm("decision")
	if decision != "approved" && decision != "rejected" {
		h.Failed(c, &service.Error{Status: 400, Message: "Некорректное решение"})
		return
	}
	if e = h.S.ReviewTask(currentPageUser(c), id, taskID, decision, c.PostForm("comment")); e != nil {
		h.Failed(c, e)
		return
	}
	c.Redirect(303, taskURL(id, taskID))
}
func (h *Pages) Admin(c *gin.Context) {
	users, e := h.S.AdminUsers(currentPageUser(c))
	if e != nil {
		h.Failed(c, e)
		return
	}
	h.Render(c, "admin", 200, gin.H{"Title": "Администрирование", "Users": users})
}
func (h *Pages) AdminRole(c *gin.Context) {
	userID, e := pageID(c, "userID")
	if e != nil {
		h.Failed(c, e)
		return
	}
	if e = h.S.AdminRole(currentPageUser(c), userID, c.PostForm("role")); e != nil {
		h.Failed(c, e)
		return
	}
	c.Redirect(303, "/admin")
}
func (h *Pages) About(c *gin.Context) { h.Render(c, "about", 200, gin.H{"Title": "Справка"}) }