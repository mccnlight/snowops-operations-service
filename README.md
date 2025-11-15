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

Все маршруты (кроме `/healthz`) требуют `Authorization: Bearer <jwt>`. Ответы оборачиваются в `{"data": ...}`. Ошибки — `{"error": "..."}` с корректным HTTP статусом (400/401/403/404/409).

### Health

#### `GET /healthz`
Проверка живости (без авторизации).

```json
{ "status": "ok" }
```

---

### Участки уборки (`/cleaning-areas`)

| Эндпоинт | Описание | Доступ |
|----------|----------|--------|
| `GET /cleaning-areas` | Список участков. Поддерживает фильтры `status`, `only_active`, `city`. | Akimat/KGU — все, Contractor — только с доступом или назначенным `default_contractor`, TOO/Drivers — 403 |
| `POST /cleaning-areas` | Создать участок. | KGU, либо Akimat если `FEATURE_ALLOW_AKIMAT_AREA_WRITE=true` |
| `GET /cleaning-areas/:id` | Детальная карточка участка. | См. список |
| `PATCH /cleaning-areas/:id` | Обновить метаданные (`name`, `description`, `status`, `default_contractor_id`). | KGU, (Akimat с флагом) |
| `PATCH /cleaning-areas/:id/geometry` | Обновить геометрию (GeoJSON). | KGU / Akimat (если флаг) |
| `GET /cleaning-areas/:id/access` | История выдач доступа подрядчикам. | KGU/Akimat (все), Contractor — только для своих участков |
| `POST /cleaning-areas/:id/access` | Выдать доступ подрядчику (`contractor_id`, `source`). | KGU |
| `DELETE /cleaning-areas/:id/access/:contractorId` | Отозвать доступ. | KGU |
| `GET /cleaning-areas/:id/ticket-template` | Возвращает участок + список доступных подрядчиков (для формы тикета). | KGU/Contractor (при наличии доступа) |

**Пример `POST /cleaning-areas`**
```bash
curl -X POST https://ops.local/cleaning-areas \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
        "name": "Мкрн. Север",
        "description": "ночная уборка",
        "geometry": "{\"type\":\"Polygon\",\"coordinates\":[[[69.15,54.88],...]]}",
        "city": "Петропавловск",
        "default_contractor_id": "3c978b8e-6c8a-4f12-8f0d-1f56d5a9d321"
      }'
```

**Пример `PATCH /cleaning-areas/:id`**
```bash
curl -X PATCH https://ops.local/cleaning-areas/96a04122-... \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
        "name": "Мкрн. Север (обновлено)",
        "status": "ACTIVE",
        "default_contractor_id": "2fd5338f-5d27-4d4f-a7f6-890abda46c1f"
      }'
```

**Пример `PATCH /cleaning-areas/:id/geometry`**
```bash
curl -X PATCH https://ops.local/cleaning-areas/96a04122-.../geometry \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
        "geometry": "{\"type\":\"Polygon\",\"coordinates\":[[[69.17,54.86],...]]}"
      }'
```

**Пример `POST /cleaning-areas/:id/access`**
```bash
curl -X POST https://ops.local/cleaning-areas/96a04122-.../access \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
        "contractor_id": "4a48f9cb-76b0-4f8c-ba69-7be79a0cbf7c",
        "source": "TICKETS"
      }'
```

---

### Полигоны и камеры (`/polygons`)

| Эндпоинт | Описание | Доступ |
|----------|----------|--------|
| `GET /polygons?only_active=true` | Список полигонов; подрядчики видят только выданные. | Akimat/KGU/TOO — все; Contractor — только доступные |
| `POST /polygons` | Создать полигон (`name`, `address`, `geometry`, `is_active`). | KGU и TOO, Akimat если `FEATURE_ALLOW_AKIMAT_POLYGON_WRITE=true` |
| `GET /polygons/:id` | Детали полигона. | Подрядчик должен иметь активный доступ |
| `PATCH /polygons/:id` | Обновить метаданные (имя, адрес, `is_active`). | KGU/TOO/(Akimat с флагом) |
| `PATCH /polygons/:id/geometry` | Обновить геометрию (GeoJSON). | KGU/TOO/(Akimat с флагом) |
| `GET /polygons/:id/access` | История доступа подрядчиков. | KGU/TOO/Akimat; Contractor — только когда имеет доступ |
| `POST /polygons/:id/access` | Выдать доступ подрядчику. | KGU/TOO |
| `DELETE /polygons/:id/access/:contractorId` | Отозвать доступ. | KGU/TOO |
| `GET /polygons/:id/cameras` | Список камер полигона. | KGU/TOO/Contractor (при доступе) |
| `POST /polygons/:id/cameras` | Создать камеру (`type`: `LPR`/`VOLUME`, `name`, `location`, `is_active`). | KGU/TOO |
| `PATCH /polygons/:id/cameras/:cameraId` | Обновить камеру. | KGU/TOO |

**Пример `POST /polygons`**
```bash
curl -X POST https://ops.local/polygons \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
        "name": "Полигон №1",
        "address": "ул. Промышленная, 5",
        "geometry": "{\"type\":\"Polygon\",\"coordinates\":[[[69.0,54.8],...]]}",
        "is_active": true
      }'
```

**Пример `PATCH /polygons/:id`**
```bash
curl -X PATCH https://ops.local/polygons/511f... \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
        "address": "ул. Промышленная, 7",
        "is_active": true
      }'
```

**Пример `POST /polygons/:id/access`**
```bash
curl -X POST https://ops.local/polygons/511f.../access \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
        "contractor_id": "4a48f9cb-76b0-4f8c-ba69-7be79a0cbf7c",
        "source": "MANUAL"
      }'
```

**Пример `POST /polygons/:id/cameras`**
```bash
curl -X POST https://ops.local/polygons/511f.../cameras \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
        "type": "LPR",
        "name": "Въезд 1",
        "location": "{\"type\":\"Point\",\"coordinates\":[69.15,54.88]}",
        "is_active": true
      }'
```

---

### Интеграции (`/integrations`)

#### `POST /integrations/polygons/:id/contains`
Проверка, входит ли точка в полигон.

```json
{ "lat": 54.88, "lng": 69.15 }
```

```json
{ "data": { "inside": true } }
```

#### `GET /integrations/cameras/:id/polygon`
Возвращает камеру и связанный полигон (нужна для LPR/volume событий).

```json
{
  "data": {
    "camera": {
      "id": "be73...",
      "polygon_id": "511f...",
      "type": "LPR",
      "location": "{\"type\":\"Point\",\"coordinates\":[69.17,54.86]}",
      "is_active": true
    },
    "polygon": {
      "id": "511f...",
      "name": "Полигон №1",
      "address": "ул. Промышленная, 5"
    }
  }
}
```

## Примечания

- Геометрия всегда передаётся/возвращается в формате GeoJSON.
- При активных доступах (тикеты/рейсы) геометрию участков менять нельзя без feature-флага.
- Таблицы `cleaning_area_access` и `polygon_access` могут наполняться вручную и автоматически (ticket/trip сервисы).
