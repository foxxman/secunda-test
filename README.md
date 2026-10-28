# Сервис управления задачами

REST API для задач внутри команд: регистрация и JWT, роли создателя, администратора и участника, история изменений, комментарии, кеш списка задач в Redis и SQL-отчет по команде.

## Команды

Развертка в фоне (backend, MySQL и Redis):

```bash
make deploy
```

Запуск на переднем плане, с логами:

```bash
make run
```

Остановка:

```bash
make down
```

Тесты, включая интеграционный тест SQL-отчета. Для интеграционного теста нужен запущенный Docker.

```bash
make test
```

`make deploy` и `make run` равнозначны `docker compose up -d --build` и `docker compose up --build`. Миграции применяются при старте backend, отдельно их запускать не нужно.

После запуска API доступен на `http://localhost:8080`. Swagger UI: `http://localhost:8080/swagger/`.

## Конфигурация

Переменные читает только процесс backend. Пароли не зашиты в код.

| Переменная | Назначение | Значение в compose |
| --- | --- | --- |
| `HTTP_ADDR` | Адрес HTTP | `:8080` |
| `MYSQL_DSN` | Подключение к MySQL | `tasks:tasks@tcp(mysql:3306)/tasks?parseTime=true&charset=utf8mb4&loc=UTC` |
| `REDIS_ADDR` | Адрес Redis | `redis:6379` |
| `REDIS_PASSWORD` | Пароль Redis | пусто |
| `JWT_SECRET` | Секрет подписи JWT, минимум 16 символов | `dev-secret-change-me`, можно переопределить снаружи |
| `JWT_TTL` | Время жизни токена | `24h` |
| `CACHE_TTL` | TTL кеша списка задач | `5m` |

Схема лежит в `internal/repository/mysql/migrations/001_init.sql`. Повторный старт не накатывает ее заново.

## Роли

Создатель команды и строка участника с ролью `owner` создаются одной транзакцией. Приглашением можно выдать только `admin` или `member`. Если пользователь уже в команде, сменить ему роль может создатель. Администратор не меняет роль создателя.

Участник создает задачи и правит те, где он создатель: название, описание, статус и исполнителя. Исполнитель меняет только статус и может комментировать. Комментарий может оставить любой участник команды. Создатель и администратор правят любые задачи своей команды и смотрят отчет.

Исполнитель должен состоять в той же команде. Статусы: `todo`, `in_progress`, `done`. При переходе в `done` ставится `closed_at`, при выходе из `done` он сбрасывается.

Список задач требует `team_id`. Ключ кеша включает команду, статус, исполнителя, `limit` и `offset`. После создания или изменения задачи кеш этой команды сбрасывается.

## Примеры

```bash
curl -s -X POST localhost:8080/api/v1/register \
  -H 'Content-Type: application/json' \
  -d '{"email":"owner@example.com","password":"password123","name":"Анна"}'

curl -s -X POST localhost:8080/api/v1/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"owner@example.com","password":"password123"}'

curl -s -X POST localhost:8080/api/v1/teams \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"name":"Платформа"}'

curl -s -X POST localhost:8080/api/v1/teams/1/invite \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"email":"member@example.com","user_id":2,"role":"member"}'

curl -s -X POST localhost:8080/api/v1/tasks \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"team_id":1,"title":"Собрать отчет","status":"todo","assignee_id":2}'

curl -s "localhost:8080/api/v1/tasks?team_id=1&status=todo&assignee_id=2&limit=20&offset=0" \
  -H "Authorization: Bearer $TOKEN"

curl -s -X PUT localhost:8080/api/v1/tasks/1 \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"status":"done","version":1}'

curl -s localhost:8080/api/v1/tasks/1/history \
  -H "Authorization: Bearer $TOKEN"

curl -s -X POST localhost:8080/api/v1/tasks/1/comments \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"content":"готово"}'

curl -s localhost:8080/api/v1/teams/1/stats \
  -H "Authorization: Bearer $TOKEN"
```

`PUT` принимает `version` задачи, которую клиент прочитал. Если строка уже изменена, сервер отвечает 409 и не записывает запрос.

Отчет доступен создателю и администратору. В нем задачи по статусам, топ-3 исполнителя по задачам, закрытым за 30 дней, среднее время закрытия по всем закрытым задачам и общее число комментариев команды. Это один SQL-запрос.
