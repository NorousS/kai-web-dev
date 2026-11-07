# Flowboard — Task Tracker

Учебный полнофункциональный MVP трекера задач для команд: Go/Gin + GORM + PostgreSQL + JavaScript + JWT + Docker + Swagger.

## Быстрый запуск

В корне репозитория:

```bash
docker compose up --build -d
```

После запуска:

- Приложение: **http://localhost:8080**
- Swagger/OpenAPI: **http://localhost:8080/swagger**
- OpenAPI YAML: **http://localhost:8080/openapi.yaml**
- Проверка сервера: **http://localhost:8080/health**

Создайте обычного пользователя через регистрацию. Для глобальной админ-панели **до первого запуска** задайте в `.env` `BOOTSTRAP_ADMIN_EMAIL` и `BOOTSTRAP_ADMIN_PASSWORD`, тогда будет создан администратор (если такого email ещё нет):

```bash
cp .env.example .env
# Отредактируйте .env: задайте уникальные пароли и JWT_SECRET >= 32 символов

docker compose up --build -d
```

> `.env.example` содержит демонстрационные строки. Внешняя публикация с dev-секретами, открытым HTTP и COOKIE_SECURE=false небезопасна. Для production настройте TLS, COOKIE_SECURE=true, секреты и ограничение доступа. Не коммитьте `.env`.

### Основной сценарий

1. Зарегистрируйте аккаунт A и создайте проект.
2. Зарегистрируйте аккаунт B, войдите в A и пригласите B по его email.
3. Владелец создаёт задачу и назначает B исполнителем.
4. B берёт задачу в работу, прикладывает описание выполненного или ссылку, отправляет «На ревью».
5. A проверяет результат, одобряет — задача получает `done` и дату завершения; либо возвращает в работу с комментарием.
6. Владелец проекта может назначить управляющего или передать владение существующему участнику.

Исполнитель **не может напрямую** присвоить задаче `done`; автор отправки **не может одобрить собственную работу**. Владелец и назначенные управляющие могут проводить ревью.

## Полезные команды

```bash
docker compose ps
docker compose logs -f api
docker compose down             # остановить контейнеры, данные сохранены
docker compose down -v          # ОПАСНО: удалить данные PostgreSQL
```

Если порт 8080 занят, в `.env` установите `APP_PORT=8081` и откройте http://localhost:8081.

## Архитектура MVC

**MVC + Service + Repository** (подробная схема: [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)).

- **Model** — `backend/internal/model`: GORM-сущности, связи, расчёт срочности; `migration` — версионированные Code First-миграции (`schema_migrations`).
- **View** — `frontend/index.html`, `frontend/app.js`, `frontend/style.css`: адаптивное JS SPA, отдаётся Nginx; `internal/view` содержит HTML-представление Swagger.
- **Controller** — `backend/internal/controller`: Gin, валидация запросов, JWT/cookies, ответы JSON.
- **Service** — `backend/internal/service`: бизнес-правила, права доступа, транзакции задач и ревью, разделённые на `service.go`, `projects.go`, `tasks.go`.
- **Repository** — `backend/internal/repository`: запросы GORM/PostgreSQL в `store.go`, `projects.go`, `tasks.go`, `dashboard.go`.
- **Router** — `backend/internal/router`: маршруты и middleware; `cmd/server` — точка входа; `cache` — TTL-кэш статистики.

```text
frontend/              View (HTML, CSS, JS)
backend/cmd/server/     Точка входа
backend/internal/
  model/                Model + бизнес-сущности
  view/                 Представление Swagger
  controller/           Controller: Gin + JSON + валидация
  service/              Use cases и права доступа
  repository/           GORM + PostgreSQL
  router/               Маршрутизация
  migration/            Версионированные миграции
  config/               Переменные окружения
  cache/                Кэш статистики
```

**Swagger:** `docs/openapi.yaml`, страница `/swagger` использует CDN Swagger UI (нужен доступ к интернету).

## Роли

**Глобальные:** `user` и `admin`. Только `admin` может открыть `/admin` и менять глобальные роли. Глобальный админ может просматривать/управлять проектами для обслуживания, но **не может удалять чужой проект**.

**В проекте:** `owner` (владелец), `manager` (управляющий), `member` (исполнитель). `owner` назначает управляющих, передаёт роль владельца; `owner` или **исходный создатель** может удалить проект. `manager` создаёт/редактирует задачи, приглашает участников и проводит ревью. `member` видит задачи проекта и может работать с назначенными ему задачами. При выходе участника его незавершённые задачи остаются, но становятся неназначенными.

## Операции и сроки

- Статусы: `todo` → `in_progress` → `in_review` → `done`, отклонённое ревью возвращает `in_progress`.
- Приоритеты: `low`, `medium`, `high`, `critical` — задаются управляющими отдельно от индикатора дедлайна.
- Автоматический индикатор: более 7 дней — зелёный, ≤7 дней — жёлтый, ≤3 дней — красный, ≤24 часов или просрочка — мигающий красный. Есть ручной `urgency_override`.
- Фильтры `/api/projects/{id}/tasks?status=&priority=&q=&assignee=` и `/api/projects?q=`.
- Сессия хранится в PostgreSQL (хэш JTI), JWT годен 7 суток, выход отзывает серверную сессию.

## Проверка

```bash
# При наличии Go и доступа к пакетам:
cd backend
go mod tidy
go test ./...
```

GitHub Actions `.github/workflows/ci.yml` выполняет `go test`, `go vet`, `go build` и проверяет синтаксис JS. Полный ручной сценарий описан в `docs/TESTING.md`.

> Миграции v1 работают через Code First/GORM с регистрацией версии схемы. Для production нужны отдельные проверенные миграции с rollback, полноценный аудит действий, rate limiting, HTTPS, резервное копирование, email-приглашения и управление восстановлением доступа.

Подробное ТЗ: [docs/TECHNICAL_SPEC.md](docs/TECHNICAL_SPEC.md).