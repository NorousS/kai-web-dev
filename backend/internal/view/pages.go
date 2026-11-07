// Package view renders complete server-side HTML pages. No JavaScript SPA is used.
package view

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"time"
)

//go:embed templates/*.html static/*.css
var assets embed.FS

type Renderer struct {
	pages map[string]*template.Template
}

func NewRenderer() (*Renderer, error) {
	funcs := template.FuncMap{
		"date": func(t *time.Time) string {
			if t == nil {
				return "Без срока"
			}
			return t.UTC().Format("02.01.2006 15:04 UTC")
		},
		"datetimeInput": func(t *time.Time) string {
			if t == nil {
				return ""
			}
			return t.UTC().Format("2006-01-02T15:04")
		},
		"status": func(s string) string {
			switch s {
			case "todo":
				return "К выполнению"
			case "in_progress":
				return "В работе"
			case "in_review":
				return "На проверке"
			case "done":
				return "Завершено"
			}
			return s
		},
		"priority": func(s string) string {
			switch s {
			case "low":
				return "Низкий"
			case "medium":
				return "Средний"
			case "high":
				return "Высокий"
			case "critical":
				return "Критический"
			}
			return s
		},
		"role": func(s string) string {
			switch s {
			case "owner":
				return "Владелец"
			case "manager":
				return "Управляющий"
			case "member":
				return "Участник"
			case "admin":
				return "Администратор"
			case "user":
				return "Пользователь"
			}
			return s
		},
		"eq":     func(a, b any) bool { return fmt.Sprint(a) == fmt.Sprint(b) },
		"sameID": func(ptr *uint, id uint) bool { return ptr != nil && *ptr == id },
	}
	pages := make(map[string]*template.Template)
	for _, name := range []string{"auth", "dashboard", "projects", "project", "task_form", "admin", "about", "error"} {
		t, err := template.New("layout.html").Funcs(funcs).ParseFS(assets, "templates/layout.html", "templates/"+name+".html")
		if err != nil {
			return nil, fmt.Errorf("template %s: %w", name, err)
		}
		pages[name] = t
	}
	return &Renderer{pages: pages}, nil
}

func (r *Renderer) Render(w http.ResponseWriter, statusCode int, page string, data any) error {
	t, ok := r.pages[page]
	if !ok {
		return fmt.Errorf("missing HTML page %q", page)
	}
	var out bytes.Buffer
	if err := t.ExecuteTemplate(&out, "layout", data); err != nil {
		return err
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; form-action 'self'; base-uri 'self'; frame-ancestors 'none'")
	w.WriteHeader(statusCode)
	_, err := w.Write(out.Bytes())
	return err
}

func Stylesheet() ([]byte, error) { return assets.ReadFile("static/style.css") }