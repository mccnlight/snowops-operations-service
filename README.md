# Snowops Operations Service

Operations-service хранит и выдаёт картографические данные: участки уборки (`cleaning_areas`), полигоны вывоза (`polygons`) и камеры (`cameras`). Сервис использует PostgreSQL + PostGIS и проверяет JWT, выпущенные `snowops-auth-service`.

## Возможности

- Управление участками уборки с учётом ролей (`AKIMAT_ADMIN`, `KGU_ZKH_ADMIN`, `CONTRACTOR_ADMIN`, `TOO_ADMIN`).
- Управление полигонами/камерами и явная выдача доступа подрядчикам через таблицы `cleaning_area_access` и `polygon_access`.
- Вспомогательные эндпоинты для интеграций (проверка GPS-попадания в полигон, быстрый `camera_id → polygon`).

## Требования

- Go 1.23+
- PostgreSQL 15+ с PostGIS

## Запуск локально

```bash
# поднять Postgres + PostGIS
cd deploy
docker compose up -d

# запустить сервис
cd ..
APP_ENV=development \
DB_DSN="postgres://postgres:postgres@localhost:5433/operations_db?sslmode=disable" \
JWT_ACCESS_SECRET="secret-key" \
go run ./cmd/operations-service
```

## Переменные окружения

| Переменная                                    | Описание                                                                 | Значение по умолчанию                                                |
|-----------------------------------------------|--------------------------------------------------------------------------|-----------------------------------------------------------------------|
| `APP_ENV`                                     | окружение (`development`, `production`)                                  | `development`                                                         |
| `HTTP_HOST` / `HTTP_PORT`                     | адрес HTTP-сервера                                                       | `0.0.0.0` / `7081`                                                    |
| `DB_DSN`                                      | строка подключения к Postgres/PostGIS                                    | `postgres://postgres:postgres@localhost:5433/operations_db?sslmode=disable` |
| `DB_MAX_OPEN_CONNS`                           | максимум одновременных соединений                                        | `25`                                                                  |
| `DB_MAX_IDLE_CONNS`                           | максимум соединений в пуле                                               | `10`                                                                  |
| `DB_CONN_MAX_LIFETIME`                        | TTL соединения                                                           | `1h`                                                                  |
| `JWT_ACCESS_SECRET`                           | секрет для проверки JWT                                                  | `supersecret`                                                         |
| `FEATURE_ALLOW_AKIMAT_AREA_WRITE`             | разрешить `AKIMAT_ADMIN` создавать/править участки                       | `false`                                                               |
| `FEATURE_ALLOW_AKIMAT_POLYGON_WRITE`          | разрешить `AKIMAT_ADMIN` управлять полигонами и камерами                 | `false`                                                               |
| `FEATURE_ALLOW_AREA_GEOMETRY_UPDATE_WHEN_IN_USE` | отключить блокировку геометрии участков с активными тикетами           | `false`                                                               |

## API

Все эндпоинты, кроме `/healthz`, требуют заголовок `Authorization: Bearer <jwt>`. Ответы по умолчанию оборачиваются в `{"data": ...}`.

### Health

#### `GET /healthz`

Проверка работоспособности (без авторизации).

```json
{ "status": "ok" }
```

---

### Участки уборки (`/cleaning-areas`)

#### `GET /cleaning-areas`
- **Описание:** список участков. Поддерживает `status=ACTIVE`, `status=INACTIVE`, `only_active=true`.
- **Доступ:** `AKIMAT_ADMIN`, `KGU_ZKH_ADMIN` (все), `CONTRACTOR_ADMIN` (только участки с доступом или назначенным подрядчиком), `TOO_ADMIN`/`DRIVER` — запрещено.

```bash
curl -H "Authorization: Bearer <token>" \
  "https://ops.local/cleaning-areas?only_active=true"
```

```json
{
  "data": [
    {
      "id": "f6c1c2f6-8d1b-4a8a-9ef4-5f2dd2a0cbaa",
      "name": "Центральный парк",
      "description": "ул. Абая",
      "geometry": "{\"type\":\"Polygon\",\"coordinates\":[...]}",
      "city": "Петропавловск",
      "status": "ACTIVE",
      "default_contractor_id": "3c978b8e-6c8a-4f12-8f0d-1f56d5a9d321",
      "is_active": true,
      "created_at": "2025-01-10T10:00:00Z",
      "updated_at": "2025-01-10T10:00:00Z"
    }
  ]
}
```

#### `POST /cleaning-areas`
- **Описание:** создать участок. `geometry` — строка GeoJSON.
- **Доступ:** `KGU_ZKH_ADMIN`, либо `AKIMAT_ADMIN` при включённом флаге.

```json
{
  "name": "Мкрн. Север",
  "description": "ул. Есиль",
  "geometry": "{\"type\":\"Polygon\",\"coordinates\":[[...]]}",
  "city": "Петропавловск",
  "default_contractor_id": "3c978b8e-6c8a-4f12-8f0d-1f56d5a9d321"
}
```

```json
{
  "data": {
    "id": "96a04122-4c63-4cda-92db-02a6a0d7d0a1",
    "name": "Мкрн. Север",
    "status": "ACTIVE",
    "...": "..."
  }
}
```

#### `GET /cleaning-areas/:id`
Возвращает участок с учётом прав доступа.

#### `PATCH /cleaning-areas/:id`
Обновление названия/описания/исполнителя/статуса. Все поля опциональны:

```json
{
  "name": "Обновлённый участок",
  "description": "Новый текст",
  "status": "INACTIVE",
  "default_contractor_id": null,
  "is_active": false
}
```

#### `PATCH /cleaning-areas/:id/geometry`
Изменение геометрии (GeoJSON). Блокируется, если есть активные доступы/тикеты и `FEATURE_ALLOW_AREA_GEOMETRY_UPDATE_WHEN_IN_USE=false`.

#### `GET /cleaning-areas/:id/access`
Список выдач доступа подрядчикам.

```json
{
  "data": {
    "access": [
      {
        "contractor_id": "3c978b8e-6c8a-4f12-8f0d-1f56d5a9d321",
        "source": "TICKETS",
        "created_at": "2025-01-12T08:00:00Z",
        "revoked_at": null
      }
    ]
  }
}
```

#### `POST /cleaning-areas/:id/access`

```json
{
  "contractor_id": "3c978b8e-6c8a-4f12-8f0d-1f56d5a9d321",
  "source": "MANUAL"
}
```

```json
{ "data": { "granted": true } }
```

#### `DELETE /cleaning-areas/:id/access/:contractorId`
Удаляет активный доступ. Ответ `204 No Content`.

#### `GET /cleaning-areas/:id/ticket-template`
Возвращает участок + список доступных подрядчиков для формы быстрого создания тикета.

```json
{
  "data": {
    "area": { "...": "..." },
    "contractors": [
      "3c978b8e-6c8a-4f12-8f0d-1f56d5a9d321",
      "2fd5338f-5d27-4d4f-a7f6-890abda46c1f"
    ]
  }
}
```

---

### Полигоны (`/polygons`)

#### `GET /polygons`
- `only_active=true` — фильтр.
- Подрядчики видят только полигоны, на которые выдан доступ.

#### `POST /polygons`

```json
{
  "name": "Полигон №1",
  "address": "ул. Промышленная, 5",
  "geometry": "{\"type\":\"Polygon\",\"coordinates\":[[...]]}",
  "is_active": true
}
```

#### `GET /polygons/:id`
Возвращает полигон. Подрядчик должен иметь активный доступ.

#### `PATCH /polygons/:id`
Обновляет имя/адрес/флаг активности.

#### `PATCH /polygons/:id/geometry`
Обновление геометрии (GeoJSON).

#### `GET /polygons/:id/access`
Получить историю доступа подрядчиков к полигону.

#### `POST /polygons/:id/access`

```json
{
  "contractor_id": "4a48f9cb-76b0-4f8c-ba69-7be79a0cbf7c",
  "source": "MANUAL"
}
```

Ответ: `{"data":{"granted":true}}`.

#### `DELETE /polygons/:id/access/:contractorId`
Удаляет доступ (204).

---

### Камеры (`/polygons/:id/cameras`)

- `GET /polygons/:id/cameras` — список камер полигона.
- `POST /polygons/:id/cameras`

```json
{
  "type": "LPR",
  "name": "Въезд 1",
  "location": "{\"type\":\"Point\",\"coordinates\":[69.15,54.88]}",
  "is_active": true
}
```

- `PATCH /polygons/:id/cameras/:cameraId` — можно менять `type`, `name`, `location`, `is_active`.

---

### Интеграции (`/integrations`)

#### `POST /integrations/polygons/:id/contains`
Проверяет, принадлежит ли GPS-точка полигону.

```json
{ "lat": 54.88, "lng": 69.15 }
```

```json
{ "data": { "inside": true } }
```

#### `GET /integrations/cameras/:id/polygon`
Возвращает камеру и связанный полигон (для LPR/volume событий).

```json
{
  "data": {
    "camera": { "id": "...", "polygon_id": "...", "type": "LPR", "...": "..." },
    "polygon": { "id": "...", "name": "Полигон №1", "...": "..." }
  }
}
```

## Примечания

- Геометрия всегда передаётся/возвращается в формате GeoJSON.
- Если `FEATURE_ALLOW_AREA_GEOMETRY_UPDATE_WHEN_IN_USE=false`, участки с активными доступами нельзя редактировать.
- Таблицы `cleaning_area_access` и `polygon_access` могут наполняться как вручную через API, так и автоматически сервисами тикетов/рейсов.
# Operations Service

Operations-service отвечает за хранение и управление геоданными: участками уборки, полигонами вывоза снега и камерами. Он использует PostgreSQL с расширением PostGIS и предоставляет REST API, защищённый JWT-токенами, выпускаемыми `auth-service`.

## Возможности

- CRUD для `cleaning_areas` с расширенным RBAC:
  - `KGU_ZKH_ADMIN` — полный доступ, выдача/отзыв доступа подрядчиков.
  - `AKIMAT_ADMIN` — read-only (можно включить права записи feature-флагом).
  - `TOO_ADMIN` — доступа нет (работает только с полигонами/камерами).
  - `CONTRACTOR_ADMIN` — видит только участки, где назначен по умолчанию или через активные тикеты/рейсы (таблица `cleaning_area_access`).
- CRUD для `polygons`/`cameras`:
  - `KGU_ZKH_ADMIN` и `TOO_ADMIN` создают и правят полигоны, управляют доступом подрядчиков (`polygon_access`).
  - `AKIMAT_ADMIN` может получить права записи через feature-флаг, иначе read-only.
  - Подрядчики видят только полигоны с выданным доступом.
- Интеграционные endpoints: проверка `polygon.contains(lat/lng)` и быстрый `camera_id → polygon` lookup для LPR/GPS систем.
- Все геометрии хранятся в PostGIS, миграции создают индексы и триггеры для новых таблиц `cleaning_area_access` и `polygon_access`.

## Запуск локально

```bash
# поднять Postgres + PostGIS
cd deploy
docker compose up -d

# запустить сервис
cd ..
APP_ENV=development \
DB_DSN="postgres://postgres:postgres@localhost:5433/operations_db?sslmode=disable" \
JWT_ACCESS_SECRET="secret-key" \
go run ./cmd/operations-service
```

### Переменные окружения

| Переменная             | Описание                                      | Значение по умолчанию              |
|------------------------|-----------------------------------------------|------------------------------------|
| `APP_ENV`              | окружение (`development`, `production`)       | `development`                      |
| `HTTP_HOST` / `HTTP_PORT` | адрес HTTP-сервера                        | `0.0.0.0:7081`                     |
| `DB_DSN`               | строка подключения к Postgres/PostGIS        | `postgres://postgres:postgres@localhost:5433/operations_db?sslmode=disable` |
| `DB_MAX_OPEN_CONNS`    | максимум открытых соединений                  | `25`                               |
| `DB_MAX_IDLE_CONNS`    | максимум соединений в пуле                    | `10`                               |
| `DB_CONN_MAX_LIFETIME` | TTL соединений                                | `1h`                               |
| `JWT_ACCESS_SECRET`    | секрет для проверки JWT из auth-service       | `supersecret`                      |
| `FEATURE_ALLOW_AKIMAT_AREA_WRITE` | разрешить акимату правки участков  | `false` |
| `FEATURE_ALLOW_AKIMAT_POLYGON_WRITE` | разрешить акимату правки полигонов/камер | `false` |
| `FEATURE_ALLOW_AREA_GEOMETRY_UPDATE_WHEN_IN_USE` | игнорировать блокировку геометрии при активных тикетах | `false` |

### Маршруты

Все маршруты (кроме `/healthz`) требуют заголовок `Authorization: Bearer <token>`.

- `GET/POST/PATCH /cleaning-areas` — стандартный CRUD.
- `PATCH /cleaning-areas/:id/geometry` — изменение геометрии (если нет активных тикетов или флаг включён).
- `GET/POST/DELETE /cleaning-areas/:id/access` — управление доступом подрядчиков к участку (используют тикеты/рейсы).
- `GET /cleaning-areas/:id/ticket-template` — данные для быстрого создания тикета из карточки участка.
- `GET/POST/PATCH /polygons` и `PATCH /polygons/:id/geometry` — управление полигонами.
- `GET/POST/DELETE /polygons/:id/access` — выдача доступа подрядчикам к полигонам.
- `GET /polygons/:id/cameras`, `POST /polygons/:id/cameras`, `PATCH /polygons/:id/cameras/:cameraId` — управление камерами.
- `POST /integrations/polygons/:id/contains` — проверка попадания координаты в полигон.
- `GET /integrations/cameras/:id/polygon` — вернуть полигон по `camera_id`.

## Примечания

- Геометрия принимается и возвращается в формате GeoJSON.
- Если у участка есть активные доступа (тикеты/рейсы), геометрия заблокирована до их завершения (если не включён feature-флаг).
- Таблицы `cleaning_area_access`/`polygon_access` могут наполняться как вручную, так и автоматически другими сервисами (тикеты, рейсы, логистика).


