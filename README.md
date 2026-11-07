# Task Tracker — серверный MVC

Учебное веб-приложение управления проектами и задачами на Go/Gin, PostgreSQL/GORM, JWT, Go `html/template`, Swagger и Docker Compose. Основной интерфейс **полностью рендерится сервером**; JavaScript для отображения страниц и отправки форм не нужен.

## Запуск

```bash
cp .env.example .env
# Замените POSTGRES_PASSWORD и JWT_SECRET; JWT_SECRET должен быть >= 32 символов.
docker compose up --build -d
```

- [Приложение](http://localhost:8080) — серверные HTML-страницы.
- [Swagger](http://localhost:8080/swagger) — документация отдельного JSON REST API.
- [Проверка работы сервера](http://localhost:8080/health).
- [OpenAPI YAML](http://localhost:8080/openapi.yaml).

Для первого администратора задайте в `.env` `BOOTSTRAP_ADMIN_EMAIL` и `BOOTSTRAP_ADMIN_PASSWORD` (от 10 символов) до запуска. Администратор создаётся, если такой email ещё не зарегистрирован.

В `.env` могут быть демонстрационные значения. Перед публикацией в интернет настройте HTTPS, `COOKIE_SECURE=true`, уникальные секреты и защиту доступа. Не загружайте `.env` в Git.

## Что изменилось относительно SPA

- Теперь `GET /login`, `/register`, `/`, `/projects`, `/projects/:id`, `/admin` возвращают **готовые HTML-документы** от Go.
- HTML-формы отправляют `application/x-www-form-urlencoded` через **POST** и получают HTTP **303 See Other** для предотвращения повторной отправки формы.
- HTML-шаблоны и CSS хранятся в Go-пакете `backend/internal/view/` и включаются в бинарный файл через `embed`.
- Nginx проксирует **все пути** в Go вместо раздачи JS SPA.
- Сессии используют JWT в HttpOnly cookie и запись в PostgreSQL; формы защищены CSRF-токеном.
- Старый REST API доступен через `/api/*` и остаётся документирован в Swagger, но **не участвует** в работе HTML-интерфейса.

## MVC и расположение файлов

```text
backend/
  cmd/server/main.go                # запуск: конфигурация, БД, миграции, сервисы
  internal/
    model/models.go                 # M — сущности GORM, приоритеты, срочность
    view/pages.go                   # V — сборка и безопасный рендер HTML
    view/templates/                # V — Go HTML-шаблоны всех страниц
      layout.html                   # общий каркас
      auth.html                     # вход и регистрация
      dashboard.html                # обзор и статистика
      projects.html                 # проекты
      project.html                  # карточка проекта, задачи и ревью
      task_form.html                # создание/редактирование задачи
      admin.html                    # админ-панель
      about.html / error.html       # справка и ошибки
    view/static/style.css          # стили (embed)
    controller/pages.go             # C — HTML-запросы, формы, cookies, 303, CSRF
    controller/{auth,projects,tasks}.go # отдельные REST-контроллеры
    service/                        # права доступа и бизнес-правила
    repository/                     # GORM/Postgres-запросы и транзакции
    migration/                      # CodeFirst и версии схемы
    router/                         # серверные и REST-маршруты
    cache/ config/                  # кэш и переменные среды
infra/nginx.conf                    # reverse proxy без отдачи SPA
Dockerfile / compose.yaml          # образы и контейнеры
```

**Путь HTML-запроса:** браузер → Nginx → Gin Router → HTML Controller → Service → Repository → PostgreSQL → Controller → Go View (`html/template`) → готовая HTML-страница → браузер.

**MVC** организует серверный код по обязанностям (Model / View / Controller). **REST** описывает сетевое взаимодействие с ресурсами через HTTP и может использоваться одновременно, например в `/api/*`. Наше основное приложение — серверный MVC, а REST API — вспомогательный интерфейс для Swagger и внешних клиентов.

Подробнее: [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

## Доска Kanban

Страница проекта выводит **четыре колонки**: «К выполнению», «В работе», «На ревью», «Завершено». Карточки распределяет Go Controller, а шаблон `backend/internal/view/templates/project.html` возвращает готовый HTML. Столбцы остаются видимыми, даже если задач нет. На мобильных устройствах доска прокручивается горизонтально. Кнопки карточек используют обычные HTML-формы с CSRF и перенаправлениями — JavaScript SPA не требуется.

## Возможности

- Регистрация, вход, выход; роли `user/admin` и `owner/manager/member` внутри проекта.
- Создание проектов, приглашения по email существующих пользователей, управление участниками и передача владения.
- Создание и назначение задач, редактирование, фильтр по статусу и названию, дедлайн (UTC), теги, эпик и приоритет.
- Автоматическая срочность: 7 дней — жёлтый, 3 дня — красный, 24 часа — мигающий красный; отображаемый приоритет повышается без перезаписи исходного.
- Исполнитель прикладывает отчёт и/или ссылку, отправляет на ревью. Управляющий, который не является автором отправки, одобряет или возвращает задачу. Прямое завершение без ревью запрещено.
- Серверная проверка прав на все операции; cookie CSRF-защита форм; серверные сессии.

## Тестирование

```bash
cd backend
go mod tidy
go test ./...
go vet ./...
```

CI GitHub Actions также собирает Docker-образ с Go. Для проверки интерфейса локально:

1. Откройте `/register`, зарегистрируйте владельца A.
2. Через `/projects` создайте проект.
3. Зарегистрируйте отдельного пользователя B (в другом браузере), затем из аккаунта A пригласите B по email.
4. Создайте задачу, назначьте B. Пользователь B отправляет отчёт на ревью; пользователь A его одобряет.
5. Проверьте `/admin` от администратора и от обычного пользователя; последний должен получить HTTP 403.

Проверить, что сервер отдаёт HTML, можно через `curl -i http://localhost:8080/login` — `Content-Type: text/html; charset=utf-8` и полноценный документ `<!doctype html>`.

Остановить контейнеры: `docker compose down` (данные БД сохраняются). `docker compose down -v` **удалит** данные PostgreSQL.