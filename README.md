# Task Service

REST API для управления задачами внутри команд. В сервисе есть:
- ролевая модель (owner / admin / member);
- история изменений задач;
- кеширование списков задач в Redis;
- SQL-отчёт по команде.

**Стек:** Go 1.26, MySQL 8.4, Redis 7, Docker Compose, Swagger.

## Быстрый старт

```bash
cp .env.example .env        # при необходимости поменяйте пароли и JWT_SECRET
docker compose up -d --build
```

| Адрес | Что там                                 |
|---|-----------------------------------------|
| http://localhost:8080/swagger/index.html | Swagger UI                              |
| http://localhost:8080/api/v1/... | REST API                                |
| http://localhost:8080/healthz | Проверка живости: пингует MySQL и Redis |

Остановить стек: `docker compose down`. Остановить и удалить данные: `docker compose down -v`.

### Состав Compose

| Сервис | Назначение |
|---|---|
| `mysql` | MySQL 8.4, данные в volume `mysql-data` |
| `redis` | Redis 7, только кеш, без persistence |
| `app` | Go API и Swagger UI на порту `APP_PORT` (по умолчанию 8080) |
| `cloudbeaver` | Веб-клиент БД (DBeaver в браузере) на http://127.0.0.1:8978/. Необязательный, профиль `tools` |

## Конфигурация

Вся конфигурация задаётся переменными окружения. В Docker Compose они берутся из `.env`.

| Переменная                                              | По умолчанию             | Описание                                                                        |
|---------------------------------------------------------|--------------------------|---------------------------------------------------------------------------------|
| **Общее**                                               |                          |                                                                                 |
| `COMPOSE_PROFILES`                                      | пусто                    | Дополнительные сервисы Compose. `tools` — запустить вместе со стеком CloudBeaver|
| `APP_PORT`                                              | `8080`                   | Порт API на хосте                                                               |
| `LOG_LEVEL`                                             | `info`                   | `debug`, `info`, `warn`, `error`                                                |
| `MIGRATE_ON_START`                                      | `true`                   | Применять миграции при старте                                                   |
| **HTTP-сервер**                                         |                          |                                                                                 |
| `HTTP_ADDR`                                             | `:8080`                  | Адрес HTTP-сервера внутри контейнера                                            |
| `HTTP_READ_HEADER_TIMEOUT` / `HTTP_READ_TIMEOUT`        | `5s` / `15s`             | Таймауты чтения заголовков и всего запроса                                      |
| `HTTP_WRITE_TIMEOUT` / `HTTP_IDLE_TIMEOUT`              | `30s` / `60s`            | Таймаут записи ответа и простоя keep-alive соединения                           |
| `HTTP_MAX_BODY_BYTES`                                   | `1048576`                | Максимальный размер тела запроса (1 МБ)                                         |
| `TRUST_PROXY_HEADERS`                                   | `false`                  | Брать IP клиента из `X-Real-IP` / `X-Forwarded-For`.                            |
| `SHUTDOWN_TIMEOUT`                                      | `15s`                    | Сколько ждать завершения запросов при остановке                                 |
| **Старт**                                               |                          |                                                                                 |
| `STARTUP_RETRY_ATTEMPTS` / `STARTUP_RETRY_INTERVAL`     | `30` / `2s`              | Ожидание MySQL при старте: число попыток и пауза между ними                     |
| **MySQL**                                               |                          |                                                                                 |
| `MYSQL_HOST_PORT`                                       | `3306`                   | Порт MySQL на хосте                                                             |
| `MYSQL_HOST` / `MYSQL_PORT`                             | `localhost` / `3306`     | Адрес MySQL                                                                     |
| `MYSQL_DATABASE` / `MYSQL_USER` / `MYSQL_PASSWORD`      | `tasks` / `tasks` / —    | База и учётные данные приложения                                                |
| `MYSQL_MAX_OPEN_CONNS` / `MYSQL_MAX_IDLE_CONNS`         | `25` / `25`              | Размер пула соединений                                                          |
| `MYSQL_CONN_MAX_LIFETIME`                               | `5m`                     | Максимальное время жизни соединения в пуле                                      |
| **Redis**                                               |                          |                                                                                 |
| `REDIS_ADDR` / `REDIS_PASSWORD` / `REDIS_DB`            | `localhost:6379` / — / `0` | Адрес Redis                                                                   |
| **JWT**                                                 |                          |                                                                                 |
| `JWT_SECRET`                                            | ≥ 32 символов            | Ключ подписи access-токенов                                                     |
| `JWT_ACCESS_TTL` / `JWT_REFRESH_TTL`                    | `15m` / `72h`            | Время жизни access- и refresh-токена                                            |
| **Кеш**                                                 |                          |                                                                                 |
| `CACHE_TASKS_TTL`                                       | `5m`                     | TTL кеша списков задач                                                          |
| `CACHE_INVALIDATE_TIMEOUT`                              | `2s`                     | Таймаут сброса кеша команды в Redis                                             |
| `CACHE_INVALIDATE_RETRY_INTERVAL`                       | `1s`                     | Как часто повторять неудавшийся сброс кеша                                      |
| `CACHE_BREAKER_FAILURES`                                | `5`                      | Сколько ошибок Redis подряд открывают circuit breaker                           |
| `CACHE_BREAKER_OPEN_TIMEOUT`                            | `10s`                    | Сколько breaker остаётся открытым перед пробным запросом                        |
| `CACHE_BREAKER_HALF_OPEN_REQUESTS`                      | `1`                      | Сколько пробных запросов пропускается в полуоткрытом состоянии                  |
| **Rate limit**                                          |                          |                                                                                 |
| `RATE_LIMIT_RPS` / `RATE_LIMIT_BURST`                   | `20` / `40`              | Лимит запросов к `/api` на один IP (token bucket)                               |
| `RATE_LIMIT_CLIENT_TTL` / `RATE_LIMIT_CLEANUP_INTERVAL` | `3m` / `1m`              | Через сколько забывать неактивный IP и как часто чистить список                 |
| **CloudBeaver**                                         |                          |                                                                                 |
| `CLOUDBEAVER_PORT`                                      | `8978`                   | Порт CloudBeaver                                                                |
| `CLOUDBEAVER_ADMIN` / `CLOUDBEAVER_ADMIN_PASSWORD`      | `cbadmin` / `ChangeMe123` | Администратор CloudBeaver: 8+ символов, разный регистр и цифра                 |

### Просмотр БД в браузере (CloudBeaver)

CloudBeaver — это веб-версия DBeaver. Он входит в необязательный профиль `tools` и по умолчанию не запускается. Включить его можно флагом или постоянно через `.env`:

```bash
docker compose --profile tools up -d     # разово
# или COMPOSE_PROFILES=tools в .env, тогда достаточно docker compose up -d
```

После запуска откройте http://127.0.0.1:8978/

## Миграции

SQL-миграции встроены в бинарник и применяются автоматически при старте (`MIGRATE_ON_START=true`).

Применённая версия хранится в таблице `migrations`.

## Тесты

```bash
go test ./...                                         # unit-тесты
go test -tags integration ./internal/repository/...   # интеграционный тест SQL-отчёта
```

Интеграционный тест через testcontainers поднимает MySQL 8.4, поэтому нужен запущенный Docker.

## Примеры запросов

```bash
API=http://localhost:8080/api/v1

# регистрация и вход
curl -s -X POST $API/register -H 'Content-Type: application/json' \
  -d '{"email":"alice@example.com","password":"Secret123","name":"Alice"}'
TOKEN=$(curl -s -X POST $API/login -H 'Content-Type: application/json' \
  -d '{"email":"alice@example.com","password":"Secret123"}' | jq -r .access_token)
AUTH="Authorization: Bearer $TOKEN"

# команда и приглашение (user_id приглашаемого должен существовать)
curl -s -X POST $API/teams -H "$AUTH" -H 'Content-Type: application/json' -d '{"name":"Backend"}'
curl -s -X POST $API/teams/1/invite -H "$AUTH" -H 'Content-Type: application/json' -d '{"user_id":2,"role":"member"}'
curl -s $API/teams -H "$AUTH"

# задачи
curl -s -X POST $API/tasks -H "$AUTH" -H 'Content-Type: application/json' \
  -d '{"team_id":1,"title":"Настроить CI","description":"GitHub Actions","assignee_id":2}'
curl -s "$API/tasks?team_id=1&status=backlog&assignee_id=2&limit=20&offset=0" -H "$AUTH"
curl -s "$API/tasks?team_id=1&limit=20&cursor=<next_cursor>" -H "$AUTH"

curl -s -X PUT $API/tasks/1 -H "$AUTH" -H 'Content-Type: application/json' \
  -d '{"title":"Настроить CI","description":"GitHub Actions","status":"in_progress","assignee_id":2,"version":1}'

# карточка задачи
curl -s $API/tasks/1 -H "$AUTH"

curl -s $API/tasks/1/history -H "$AUTH"
curl -s -X POST $API/tasks/1/comments -H "$AUTH" -H 'Content-Type: application/json' -d '{"content":"Взял в работу"}'
curl -s $API/tasks/1/comments -H "$AUTH"

# отчёт (owner/admin)
curl -s $API/teams/1/stats -H "$AUTH"

# обновление токенов
curl -s -X POST $API/refresh -H 'Content-Type: application/json' -d '{"refresh_token":"<refresh_token>"}'
```

## Принятые решения

- **Статусы задач:** `backlog`, `in_progress`, `need_info`, `done`. Новая задача всегда создаётся в `backlog`
- **Исполнитель** — любой участник команды.
- **Удаление задачи:** owner, admin и создатель. История и комментарии удаляются каскадно.
- **Удаление команды:** только owner.
- **История:** `changes` хранится как JSON `{"field": {"old": ..., "new": ...}}`. Создание задачи тоже пишется в историю.
- **Пагинация:** если передан `cursor`, используется он, иначе `limit`/`offset`.

### Валидация
Все текстовые поля ограничены лимитами. Длина считается в символах.
| Поле | Лимит | Правила |
|---|---|---|
| `name` пользователя | 2–256 | Буквы любого алфавита, пробел, дефис, апостроф, точка. Начинается с буквы, без двойных пробелов |
| `email` | ≤ 256 | Валидный адрес, локальная часть ≤ 64, домен с буквенным TLD ≥ 2 символов. Приводится к нижнему регистру |
| `password` | ≥ 8 символов | Строчная и заглавная буква, цифра; без пробелов. |
| `name` команды, `title` задачи | 1–256 | Одна строка, без управляющих символов |
| `description`, комментарий | ≤ 512 (комментарий ≥ 1) | Переводы строк и табуляция разрешены, другие управляющие символы нет |


### Пагинация
Оба режима сортируют задачи по `id DESC` (от новых к старым).
- **offset**: `limit` + `offset`, в ответе приходят `total` и `offset`.
- **cursor**: `limit` + `cursor`. Курсор непрозрачный (base64 от id последней задачи), в запросе используется `WHERE id < ?`. `next_cursor` возвращается в обоих режимах; `null` означает, что страниц больше нет.


### Аутентификация
- Access-токен — JWT (HS256), в `sub` лежит id пользователя.
- Refresh-токен — случайные 32 байта, в БД хранится только SHA-256 от него. При каждом `/refresh` выполняется ротация: старый токен отзывается, выдаётся новая пара. `/logout` отзывает токен.
- Пароли хранятся как bcrypt-хеши.