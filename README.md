# Snowops Operations Service

Operations-service хранит и выдаёт картографические данные: участки уборки (`cleaning_areas`), полигоны вывоза (`polygons`) и камеры (`cameras`). Сервис использует PostgreSQL + PostGIS и проверяет JWT, выпущенные `snowops-auth-service`.

## Возможности

- Управление участками уборки с учётом ролей (`AKIMAT_ADMIN`, `KGU_ZKH_ADMIN`, `CONTRACTOR_ADMIN`, `LANDFILL_ADMIN`).
- Управление полигонами/камерами и явная выдача доступа подрядчикам через `cleaning_area_access` и `polygon_access`.
- Полигоны могут быть привязаны к LANDFILL организациям через поле `organization_id`.
- Интеграционные эндпоинты: `polygon.contains(lat/lng)` и `camera_id → polygon` для LPR/volume систем.
- **Мониторинг техники в реальном времени**: отображение положения транспортных средств на карте с GPS-треками.
- **Онлайн-локации водителей**: сохранение текущей координаты с фронтенда и выдача данных для Akimat/KGU и самих водителей.
- **GPS-симулятор**: имитация движения техники по дорогам OSM со скоростью 20 км/ч для тестирования без реальных GPS-устройств.

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
| `GPS_SIMULATOR_ENABLED` | включить GPS-симулятор | `true` (development), `false` (production) |
| `GPS_SIMULATOR_INTERVAL` | интервал обновления GPS-точек | `5s` |
| `GPS_SIMULATOR_CLEANUP_DAYS` | автоматически удалять точки старше N дней (0 = отключено) | `7` |

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
| `GET /cleaning-areas/:id/deletion-info` | Получить информацию о связанных данных перед удалением. | KGU, (Akimat с флагом) |
| `DELETE /cleaning-areas/:id?force=true` | Удалить участок. Без `force` нельзя удалить, если есть связанные тикеты. С `force=true` удаляет все связанные данные каскадно. | KGU, (Akimat с флагом) |
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

**Пример `GET /cleaning-areas/:id/deletion-info`**
```bash
curl -X GET https://ops.local/cleaning-areas/96a04122-.../deletion-info \
  -H "Authorization: Bearer <token>"
```

Ответ показывает все связанные данные:
```json
{
  "data": {
    "area": {
      "id": "96a04122-...",
      "name": "Мкрн. Север"
    },
    "dependencies": {
      "tickets_count": 5,
      "trips_count": 12,
      "assignments_count": 8,
      "appeals_count": 2,
      "violations_count": 3,
      "access_records_count": 4
    },
    "will_be_deleted": {
      "tickets": true,
      "trips": true,
      "assignments": true,
      "appeals": true,
      "violations": true,
      "access_records": true
    }
  }
}
```

**Пример `DELETE /cleaning-areas/:id` (без force)**
```bash
curl -X DELETE https://ops.local/cleaning-areas/96a04122-... \
  -H "Authorization: Bearer <token>"
```

**Пример `DELETE /cleaning-areas/:id?force=true` (каскадное удаление)**
```bash
curl -X DELETE "https://ops.local/cleaning-areas/96a04122-...?force=true" \
  -H "Authorization: Bearer <token>"
```

**Ошибки:**
- `409 Conflict` — участок нельзя удалить, так как есть связанные тикеты (если `force` не указан)
- `404 Not Found` — участок не найден
- `403 Forbidden` — недостаточно прав

**Примечание:** При `force=true` удаляются все связанные данные:
- **Тикеты** (tickets) — удаляются
- **Назначения водителей** (ticket_assignments) — удаляются каскадно
- **Апелляции** (appeals) — удаляются каскадно
- **Рейсы** (trips) — `ticket_id` становится NULL
- **Нарушения** (violations) — остаются, но теряют связь с тикетами через рейсы
- **Записи доступа** (cleaning_area_access) — удаляются каскадно

---

### Полигоны и камеры (`/polygons`)

| Эндпоинт | Описание | Доступ |
|----------|----------|--------|
| `GET /polygons?only_active=true` | Список полигонов; подрядчики видят только выданные; LANDFILL видит только свои полигоны. | Akimat/KGU/LANDFILL — все; Contractor — только доступные; LANDFILL — только свои (organization_id) |
| `POST /polygons` | Создать полигон (`name`, `address`, `geometry`, `organization_id`, `is_active`). | KGU, LANDFILL_ADMIN, LANDFILL_USER; Akimat если `FEATURE_ALLOW_AKIMAT_POLYGON_WRITE=true` |
| `GET /polygons/:id` | Детали полигона. | Подрядчик должен иметь активный доступ; LANDFILL — только свои полигоны |
| `PATCH /polygons/:id` | Обновить метаданные (имя, адрес, `is_active`). | KGU/LANDFILL/(Akimat с флагом) |
| `PATCH /polygons/:id/geometry` | Обновить геометрию (GeoJSON). | KGU/LANDFILL/(Akimat с флагом) |
| `DELETE /polygons/:id` | Удалить полигон. Нельзя удалить, если есть связанные рейсы. Камеры и доступы удалятся автоматически. | KGU/LANDFILL/(Akimat с флагом) |
| `GET /polygons/:id/access` | История доступа подрядчиков. | KGU/LANDFILL/Akimat; Contractor — только когда имеет доступ |
| `POST /polygons/:id/access` | Выдать доступ подрядчику. | KGU/LANDFILL |
| `DELETE /polygons/:id/access/:contractorId` | Отозвать доступ. | KGU/LANDFILL |
| `GET /polygons/:id/cameras` | Список камер полигона. | KGU/LANDFILL/Contractor (при доступе) |
| `POST /polygons/:id/cameras` | Создать камеру (`type`: `LPR`/`VOLUME`, `name`, `location`, `is_active`). | KGU/LANDFILL |
| `PATCH /polygons/:id/cameras/:cameraId` | Обновить камеру. | KGU/LANDFILL |
| `DELETE /polygons/:id/cameras/:cameraId` | Удалить камеру. | KGU/LANDFILL |

**Примечание:** Роль `TOO_ADMIN` помечена как deprecated и заменена на `LANDFILL_ADMIN` и `LANDFILL_USER`. Для обратной совместимости `TOO_ADMIN` все еще работает, но рекомендуется использовать новые роли.

**Пример `POST /polygons`**
```bash
curl -X POST https://ops.local/polygons \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
        "name": "Полигон №1",
        "address": "ул. Промышленная, 5",
        "geometry": "{\"type\":\"Polygon\",\"coordinates\":[[[69.0,54.8],...]]}",
        "organization_id": "uuid",
        "is_active": true
      }'
```

**Примечание:** Поле `organization_id` опционально. Для LANDFILL организаций оно устанавливается автоматически из JWT токена. KGU может указать `organization_id` для привязки полигона к LANDFILL организации.

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

**Пример `DELETE /polygons/:id`**
```bash
curl -X DELETE https://ops.local/polygons/511f... \
  -H "Authorization: Bearer <token>"
```

**Ошибки:**
- `409 Conflict` — полигон нельзя удалить, так как есть связанные рейсы
- `404 Not Found` — полигон не найден
- `403 Forbidden` — недостаточно прав

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

**Пример `DELETE /polygons/:id/cameras/:cameraId`**
```bash
curl -X DELETE https://ops.local/polygons/511f.../cameras/be73... \
  -H "Authorization: Bearer <token>"
```

**Ошибки:**
- `404 Not Found` — камера не найдена
- `403 Forbidden` — недостаточно прав

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

---

## Мониторинг (`/monitoring`)

### `GET /monitoring/vehicles-live`

Возвращает список техники с последними GPS-координатами в реальном времени.

**Параметры запроса:**
- `min_lat`, `min_lon`, `max_lat`, `max_lon` (опционально) — ограничение по bounding box
- `contractor_id` (опционально) — фильтр по подрядчику

**Права доступа:**
- `AKIMAT_ADMIN`, `KGU_ZKH_ADMIN` — видят все машины
- `LANDFILL_ADMIN`, `LANDFILL_USER` — видят все машины (но не участки)
- `TOO_ADMIN` — видят все машины (но не участки) (deprecated, используйте LANDFILL_ADMIN)
- `CONTRACTOR_ADMIN` — видят только свои машины
- `DRIVER` — видят только машины, связанные с их тикетами

**Пример ответа:**
```json
{
  "data": {
    "timestamp": "2025-11-16T18:21:05Z",
    "vehicles": [
      {
        "vehicle_id": "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
        "plate_number": "KZ 123 ABC",
        "contractor_id": "1111-2222-...",
        "last_gps": {
          "lat": 54.882345,
          "lon": 69.157890,
          "captured_at": "2025-11-16T18:21:03Z",
          "speed_kmh": 19.7,
          "heading_deg": 45.3,
          "is_simulated": true
        },
        "status": "IN_TRIP"
      }
    ]
  }
}
```

### `GET /monitoring/vehicles/:id/track`

Возвращает трек (историю GPS-точек) для указанной машины за период.

**Параметры запроса:**
- `from` (опционально) — начало периода в формате RFC3339 (по умолчанию: последний час)
- `to` (опционально) — конец периода в формате RFC3339 (по умолчанию: текущее время)

**Пример ответа:**
```json
{
  "data": {
    "vehicle_id": "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
    "from": "2025-11-16T18:00:00Z",
    "to": "2025-11-16T18:30:00Z",
    "points": [
      {
        "lat": 54.880100,
        "lon": 69.150000,
        "captured_at": "2025-11-16T18:00:01Z",
        "speed_kmh": 18.5,
        "heading_deg": 90.0
      }
    ]
  }
}
```

### `DELETE /monitoring/gps-points`

Удаляет GPS-точки старше указанной даты. Используется для очистки старых данных и управления размером базы данных.

**Права доступа:**
- `KGU_ZKH_ADMIN`, `AKIMAT_ADMIN` — могут удалять GPS-точки

**Параметры запроса (один из двух обязателен):**
- `older_than` (опционально) — удалить точки старше указанной даты в формате RFC3339
- `days` (опционально) — удалить точки старше N дней (альтернативный способ)

**Пример с `older_than`:**
```bash
curl -X DELETE "https://ops.local/monitoring/gps-points?older_than=2024-01-01T00:00:00Z" \
  -H "Authorization: Bearer <token>"
```

**Пример с `days`:**
```bash
curl -X DELETE "https://ops.local/monitoring/gps-points?days=30" \
  -H "Authorization: Bearer <token>"
```

**Пример ответа:**
```json
{
  "data": {
    "deleted_count": 1250,
    "cutoff_time": "2024-01-01T00:00:00Z"
  }
}
```

**Ошибки:**
- `400 Bad Request` — не указан параметр `older_than` или `days`, либо неверный формат
- `403 Forbidden` — недостаточно прав
- `400 Bad Request` — указана дата в будущем (для `older_than`)

**Примечание:** Этот эндпоинт полезен для:
- Ручной очистки старых данных администраторами
- Интеграции с внешними cron-задачами для автоматической очистки
- Управления размером базы данных

GPS-симулятор также автоматически очищает старые точки (настраивается через `GPS_SIMULATOR_CLEANUP_DAYS`), но этот эндпоинт позволяет выполнять очистку вручную или по расписанию.

---

## Водители (`/drivers`)

### `POST /drivers/location`
Водитель отправляет текущую координату. Храним только последнюю точку (без истории).

```bash
curl -X POST https://ops.local/drivers/location \
  -H "Authorization: Bearer <driver_token>" \
  -H "Content-Type: application/json" \
  -d '{ "lat": 54.8429, "lon": 69.2071, "accuracy": 12.5 }'
```

### `GET /drivers/locations`
- `AKIMAT_ADMIN`, `KGU_ZKH_ADMIN` — видят всех водителей
- `DRIVER` — видит только себя

```json
{
  "data": {
    "locations": [
      {
        "driver_id": "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
        "lat": 54.8429,
        "lon": 69.2071,
        "accuracy": 12.5,
        "updated_at": "2025-11-16T18:34:55Z"
      }
    ]
  }
}
```

---

## GPS-симулятор

Сервис включает встроенный GPS-симулятор, который имитирует движение техники по дорогам OSM. Симулятор:

- Загружает дороги из файла `kz_bbox.pbf` (OSM PBF формат)
- Выбирает случайную дорогу типа `highway=primary`
- Генерирует GPS-точки с настраиваемым интервалом (по умолчанию 5 секунд) со скоростью 20 км/ч
- Сохраняет точки в таблицу `gps_points` с пометкой `simulated: true`
- Автоматически очищает старые точки (настраивается через `GPS_SIMULATOR_CLEANUP_DAYS`)

**Настройки:**
- `GPS_SIMULATOR_ENABLED=true` — включить/выключить симулятор
- `GPS_SIMULATOR_INTERVAL=5s` — интервал обновления (рекомендуется 5-10 секунд для снижения нагрузки на БД)
- `GPS_SIMULATOR_CLEANUP_DAYS=7` — автоматически удалять точки старше 7 дней (0 = отключено)

**Примечание:** Для MVP используется упрощённый парсер OSM. В продакшене рекомендуется использовать полноценную библиотеку для парсинга OSM PBF (например, `github.com/qedus/osm`).

**Формат данных:** GPS-точки от симулятора неотличимы от реальных GPS-данных на уровне БД и API. При подключении реального GPS-провайдера симулятор можно просто отключить (`GPS_SIMULATOR_ENABLED=false`).

---

## Примечания

- Геометрия всегда передаётся/возвращается в формате GeoJSON.
- При активных доступах (тикеты/рейсы) геометрию участков менять нельзя без feature-флага.
- Таблицы `cleaning_area_access` и `polygon_access` могут наполняться вручную и автоматически (ticket/trip сервисы).
- GPS-симулятор автоматически запускается при старте сервиса, если файл `kz_bbox.pbf` доступен.
- Статусы техники: `IN_TRIP` (активное движение, точка < 2 мин), `IDLE` (простой, точка 2-5 мин), `OFFLINE` (нет данных > 5 мин).
