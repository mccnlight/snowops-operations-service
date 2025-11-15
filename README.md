# Snowops Operations Service

Operations-service хранит и выдаёт картографические данные: участки уборки (`cleaning_areas`), полигоны вывоза (`polygons`) и камеры (`cameras`). Сервис использует PostgreSQL + PostGIS и проверяет JWT, выпущенные `snowops-auth-service`.

## Возможности

- Управление участками уборки с учётом ролей (`AKIMAT_ADMIN`, `KGU_ZKH_ADMIN`, `CONTRACTOR_ADMIN`, `TOO_ADMIN`).
- Управление полигонами/камерами и явная выдача доступа подрядчикам через `cleaning_area_access` и `polygon_access`.
- Интеграционные эндпоинты: `polygon.contains(lat/lng)` и `camera_id → polygon` для LPR/volume систем.

## Требования

- Go 1.23+
- PostgreSQL 15+ с PostGIS

## Запуск локально

```bash
cd deploy
docker compose up -d  # Postgres + PostGIS

cd ..
APP_ENV=development \
DB_DSN="postgres://postgres:postgres@localhost:5433/operations_db?sslmode=disable" \
JWT_ACCESS_SECRET="secret-key" \
go run ./cmd/operations-service
```

## Переменные окружения

| Переменная | Описание | Значение по умолчанию |
|------------|----------|------------------------|
| `APP_ENV` | окружение (`development`, `production`) | `development` |
| `HTTP_HOST` / `HTTP_PORT` | адрес HTTP-сервера | `0.0.0.0` / `7081` |
| `DB_DSN` | строка подключения к Postgres/PostGIS | `postgres://postgres:postgres@localhost:5433/operations_db?sslmode=disable` |
| `DB_MAX_OPEN_CONNS` / `DB_MAX_IDLE_CONNS` | предел соединений | `25` / `10` |
| `DB_CONN_MAX_LIFETIME` | TTL соединения | `1h` |
| `JWT_ACCESS_SECRET` | секрет для проверки JWT | `supersecret` |
| `FEATURE_ALLOW_AKIMAT_AREA_WRITE` | разрешить акимату править участки | `false` |
| `FEATURE_ALLOW_AKIMAT_POLYGON_WRITE` | разрешить акимату править полигоны/камеры | `false` |
| `FEATURE_ALLOW_AREA_GEOMETRY_UPDATE_WHEN_IN_USE` | позволить менять геометрию при активных доступаx | `false` |

## API

Все маршруты (кроме `/healthz`) требуют `Authorization: Bearer <jwt>`. Ответы оборачиваются в `{"data": ...}`.

### Health

- `GET /healthz` → `{ "status": "ok" }`

### Участки уборки (`/cleaning-areas`)

- `GET /cleaning-areas` — фильтры `status`, `only_active`, `city`. Доступ: KGU/Akimat (все), Contractor (только назначенные/выданные), TOO/Driver — 403.
- `POST /cleaning-areas` — создать участок (KGU, Akimat c feature-флагом).
- `GET /cleaning-areas/:id` — карточка участка с учётом доступа.
- `PATCH /cleaning-areas/:id` — обновить метаданные.
- `PATCH /cleaning-areas/:id/geometry` — обновить геометрию (запрещено, если есть активные доступы и флаг выключен).
- `GET /cleaning-areas/:id/access` — история выдач. `POST`/`DELETE` — управление доступом подрядчиков.
- `GET /cleaning-areas/:id/ticket-template` — участок + список подрядчиков для быстрого создания тикетов.

### Полигоны (`/polygons`)

- `GET /polygons` — подрядчики видят только доступные им полигоны.
- `POST /polygons`, `PATCH /polygons/:id`, `PATCH /polygons/:id/geometry` — CRUD. Доступ: KGU + TOO (Akimat — только с feature-флагом).
- `GET/POST/DELETE /polygons/:id/access` — выдача доступа подрядчикам.
- `GET /polygons/:id/cameras`, `POST /polygons/:id/cameras`, `PATCH /polygons/:id/cameras/:cameraId` — управление камерами.

### Интеграции (`/integrations`)

- `POST /integrations/polygons/:id/contains` — проверить, попадает ли GPS точка в полигон.
- `GET /integrations/cameras/:id/polygon` — вернуть камеру и связанный полигон (LPR/volume).

## Примечания

- Геометрия всегда передаётся/возвращается в формате GeoJSON.
- При активных доступах (тикеты/рейсы) геометрию участков менять нельзя без feature-флага.
- Таблицы `cleaning_area_access` и `polygon_access` могут наполняться вручную и автоматически (ticket/trip сервисы).
