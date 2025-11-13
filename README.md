# Operations Service

Operations-service отвечает за хранение и управление геоданными: участками уборки, полигонами вывоза снега и камерами. Он использует PostgreSQL с расширением PostGIS и предоставляет REST API, защищённый JWT-токенами, выпускаемыми `auth-service`.

## Возможности

- CRUD для `cleaning_areas` (участки уборки) с контролем доступа:
  - Акимат — полный доступ, включая изменение геометрии.
  - ТОО — создание и редактирование метаданных своих участков, геометрия только для чтения.
  - Подрядчики — просмотр участков, привязанных к ним.
- CRUD для `polygons` (полигоны вывоза) и камер:
  - Акимат управляет полигонами и списком камер.
  - ТОО и подрядчики видят перечень полигонов и камер.
- Миграции создают таблицы с типами `geometry` (POLYGON/POINT) и индексами PostGIS.

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
| `JWT_ACCESS_SECRET`   | секрет для проверки JWT из auth-service       | `supersecret`                      |

### Маршруты

Все маршруты (кроме `/healthz`) требуют заголовок `Authorization: Bearer <token>`.

- `GET /cleaning-areas` — список участков, фильтры `status` и `only_active`.
- `POST /cleaning-areas` — создать участок (Акимат, ТОО).
- `PATCH /cleaning-areas/:id` — обновить метаданные.
- `PATCH /cleaning-areas/:id/geometry` — обновить геометрию (только Акимат).
- `GET /polygons` / `POST /polygons` / `PATCH /polygons/:id` / `PATCH /polygons/:id/geometry`.
- `GET /polygons/:id/cameras` — список камер полигона.
- `POST /polygons/:id/cameras`, `PATCH /polygons/:id/cameras/:cameraId` — управление камерами (Акимат).

## Примечания

- Геометрия принимается и возвращается в формате GeoJSON.
- Перед изменением геометрии рекомендуется убедиться, что нет активных тикетов, чтобы не нарушить привязки (логика в сервисах тикетов на следующих этапах).


