package main

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"net/mail"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"
	"taskmanager/internal/model"
	"taskmanager/internal/web"
	"taskmanager/migrations"
)

func main() {
	if err := run(); err != nil {
		slog.Error("Ошибка запуска", "error", err)
		os.Exit(1)
	}
}

func run() error {
	command := "serve"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return errors.New("DATABASE_URL обязателен")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if command == "migrate" {
		db, err := sql.Open("pgx", url)
		if err != nil {
			return err
		}
		defer db.Close()
		goose.SetBaseFS(migrations.Files)
		if err = goose.SetDialect("postgres"); err != nil {
			return err
		}
		return goose.UpContext(ctx, db, ".")
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return err
	}
	defer pool.Close()
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	err = pool.Ping(pingCtx)
	cancel()
	if err != nil {
		return fmt.Errorf("подключение БД: %w", err)
	}
	store := model.New(pool)
	switch command {
	case "create-admin":
		return createAdmin(ctx, store, os.Args[2:])
	case "serve":
		secure, _ := strconv.ParseBool(os.Getenv("COOKIE_SECURE"))
		router := web.New(store, secure)
		srv := &http.Server{Addr: ":8080", Handler: router, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}
		done := make(chan error, 1)
		go func() { done <- srv.ListenAndServe() }()
		slog.Info("Task Manager запущен", "address", ":8080")
		select {
		case err = <-done:
			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}
			return err
		case <-ctx.Done():
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			return srv.Shutdown(shutdownCtx)
		}
	default:
		return fmt.Errorf("неизвестная команда %q", command)
	}
}

func createAdmin(ctx context.Context, store *model.Store, args []string) error {
	flags := flag.NewFlagSet("create-admin", flag.ContinueOnError)
	username := flags.String("username", "", "Имя администратора")
	email := flags.String("email", "", "Email администратора")
	if err := flags.Parse(args); err != nil {
		return err
	}
	name := strings.TrimSpace(*username)
	address := strings.ToLower(strings.TrimSpace(*email))
	parsed, err := mail.ParseAddress(address)
	if utf8.RuneCountInString(name) < 3 || utf8.RuneCountInString(name) > 50 || err != nil || parsed.Address != address || utf8.RuneCountInString(address) > 100 {
		return errors.New("проверьте имя (3–50) и email (до 100)")
	}
	var password []byte
	if term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprint(os.Stderr, "Пароль: ")
		password, err = term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
	} else {
		var line string
		line, err = bufio.NewReader(os.Stdin).ReadString('\n')
		if errors.Is(err, os.ErrClosed) {
			return err
		}
		if len(line) > 0 {
			err = nil
		}
		password = []byte(strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r"))
	}
	if err != nil {
		return fmt.Errorf("чтение пароля: %w", err)
	}
	if utf8.RuneCount(password) < 8 || len(password) > 72 {
		return errors.New("пароль: от 8 символов до 72 байт")
	}
	hash, err := bcrypt.GenerateFromPassword(password, bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	// Erase the mutable input buffer after hashing.
	for i := range password {
		password[i] = 0
	}
	_, err = store.CreateUser(ctx, name, address, string(hash), "admin")
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stdout, "Администратор создан")
	return nil
}
