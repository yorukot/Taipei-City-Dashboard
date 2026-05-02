# Docker Usage

This project is split into frontend, backend, database/cache/vector services, and one-time init jobs. For local development, use Docker Compose. Plain `docker run` is useful only when you already have the dependent services running.

## Local Environment

The local Compose stack reads `docker/.env`.

If it does not exist, create it from the template:

```sh
cp docker/.env.template docker/.env
```

Fill these values before the first boot:

```env
DB_DASHBOARD_PASSWORD=dashboard_local_password
DB_MANAGER_PASSWORD=manager_local_password
DB_MANAGER_PUBLISHED_PORT=15432
JWT_SECRET=local_dev_jwt_secret_change_me
IDNO_SALT=local_dev_idno_salt_change_me
DASHBOARD_DEFAULT_USERNAME=admin
DASHBOARD_DEFAULT_Email=admin@example.com
DASHBOARD_DEFAULT_PASSWORD=admin123456
QDRANT_API_KEY=local_qdrant_api_key
VITE_MAPBOXTOKEN=your_mapbox_public_token
VITE_MAPBOXTILE=mapbox://your.tileset
VITE_PERSONAL_BOARD_UPDATE=
```

`docker/.env` and `Taipei-City-Dashboard-FE/.env` are ignored by git. Do not commit real secrets.

## Docker Compose

Create the external Docker network once:

```sh
docker network create --driver=bridge --subnet=192.168.128.0/24 --gateway=192.168.128.1 br_dashboard
```

Start database, Redis, Qdrant, and pgAdmin:

```sh
docker compose --env-file docker/.env -f docker/docker-compose-db.yaml up -d
```

Install frontend dependencies:

```sh
docker compose --env-file docker/.env -f docker/docker-compose-init.yaml up dashboard-fe-init
```

Initialize backend sample data and the admin user:

```sh
docker compose --env-file docker/.env \
  -f docker/docker-compose-db.yaml \
  -f docker/docker-compose-init.yaml \
  up dashboard-be-init-manager dashboard-be-init-dashboard
```

Start the dashboard frontend and backend:

```sh
docker compose --env-file docker/.env -f docker/docker-compose.yaml up -d dashboard-be dashboard-fe
```

Open:

- Frontend: `http://localhost:8080`
- Backend API: `http://localhost:8088/api/v1`
- pgAdmin: `http://localhost:8889`
- Manager DB host port: `15432`
- Qdrant: `http://localhost:6333`

Check status and logs:

```sh
docker compose --env-file docker/.env -f docker/docker-compose-db.yaml -f docker/docker-compose.yaml ps
docker logs -f dashboard-fe
docker logs -f dashboard-be
```

Stop app services:

```sh
docker compose --env-file docker/.env -f docker/docker-compose.yaml stop dashboard-fe dashboard-be
```

Stop everything from this stack:

```sh
docker compose --env-file docker/.env -f docker/docker-compose.yaml -f docker/docker-compose-db.yaml down
```

## Docker Run

Use `docker run` only after the `br_dashboard` network and dependency services are already running.

Build the backend dev image:

```sh
docker build -t dashboard-be-dev:latest --target dev Taipei-City-Dashboard-BE
```

Run the backend:

```sh
docker run --rm --name dashboard-be \
  --network br_dashboard \
  -p 8088:8080 \
  --env-file docker/.env \
  -v "$PWD/Taipei-City-Dashboard-BE:/opt/Taipei-City-Dashboard-BE" \
  -w /opt/Taipei-City-Dashboard-BE \
  dashboard-be-dev:latest \
  go run main.go
```

Run the frontend dev server:

```sh
docker run --rm --name dashboard-fe \
  --network br_dashboard \
  -p 8080:80 \
  --env-file docker/.env \
  -e DOCKER_COMPOSE=true \
  -v "$PWD/Taipei-City-Dashboard-FE:/opt/Taipei-City-Dashboard-FE" \
  -w /opt/Taipei-City-Dashboard-FE \
  node:21.6.0-alpine3.18 \
  npm run dev
```

## Validation

```sh
curl -I http://127.0.0.1:8080
curl -s -o /tmp/dashboard-api.json -w '%{http_code}\n' http://127.0.0.1:8088/api/v1/dashboard/
curl -s -o /tmp/dashboard-proxy.json -w '%{http_code}\n' http://127.0.0.1:8080/api/dev/dashboard/
```

Expected result: all three checks return HTTP `200`.
