# AGENTS.md

## Repo layout

Three fully independent sub-projects — no shared workspace, no root-level package manager.

| Directory | Stack | Role |
|---|---|---|
| `Taipei-City-Dashboard-FE/` | Vue 3 + Vite + Pinia, JavaScript | Frontend SPA |
| `Taipei-City-Dashboard-BE/` | Go 1.24 + Gin + GORM | REST API backend |
| `Taipei-City-Dashboard-DE/` | Python + Apache Airflow | ETL pipelines |
| `docker/` | Docker Compose + Nginx | Full-stack local dev |
| `helm-chart/` | Helm | Kubernetes deployment |
| `db-sample-data/` | SQL | DB seed data |

---

## Frontend (`Taipei-City-Dashboard-FE/`)

**Package manager: pnpm** (Node 22 required, pnpm version pinned in `package.json#packageManager`).

```bash
pnpm install --frozen-lockfile   # always use --frozen-lockfile
pnpm dev                         # dev server on port 80
pnpm build                       # runs `eslint --fix` THEN vite build — lint errors block builds
pnpm build:test                  # vite build --mode test
pnpm lint                        # eslint . --fix
pnpm format                      # prettier --write .
pnpm exec prettier --check .     # what CI runs (does not write)
```

**No test runner is configured** — there is no vitest, jest, or cypress setup.

### Style quirks (non-negotiable, CI/lint enforced)
- **Indentation: tabs**, not spaces. ESLint rule `"indent": ["error", "tab"]` will error on spaces.
- **Double quotes** — Prettier `"singleQuote": false`.
- **Semicolons required** — Prettier `"semi": true`.
- **`console.log` is banned** — only `console.warn` and `console.error` are allowed. ESLint will error.
- **Object destructuring required** — `"prefer-destructuring": ["error", { object: true, array: false }]`. Array destructuring is optional.
- **`no-unused-vars` is an error** — exception pattern: `req|res|next|val|err`.
- `prettier-plugin-organize-imports` auto-sorts imports on `pnpm format`.
- No TypeScript — pure JavaScript throughout. Vue 3 `<script setup>` style.
- `quotes` and `semi` rules are `"off"` in ESLint — Prettier owns them entirely.

### CI gates (frontend)
1. `pnpm exec prettier --check .` — fails if any file is unformatted.
2. `pnpm build` — ESLint fix + Vite production build.

### Environment
Requires `Taipei-City-Dashboard-FE/.env`. Critical variables:

| Variable | Purpose |
|---|---|
| `VITE_MAPBOXTOKEN` | Mapbox GL access token — maps won't render without it |
| `VITE_MAPBOXTILE` | Mapbox vector tile URL for 3D Taipei buildings |
| `VITE_API_URL` | axios `baseURL` (default `/api`) |
| `VITE_APP_TITLE` / `VITE_APP_VERSION` | Display metadata |
| `VITE_TAIPEIPASS_*` | TaipeiPass OAuth2 — only needed for city gov SSO |
| `VITE_PERSONAL_BOARD_UPDATE` | Comma-separated `id:intervalMs` pairs for auto-refresh |

### Vite proxy rules

**Without `DOCKER_COMPOSE=true` (default):**
- `/api/*` → `https://citydashboard.taipei/api/v1/*`
- `/geo_server/*` → `https://citydashboard.taipei/geo_server/*`

**With `DOCKER_COMPOSE=true`:**
- `/api/dev/*` → `http://dashboard-be:8080/v1/*` (replaces `/dev` with `/v1`)
- No geo_server proxy

### Path aliases
**No `@` alias is defined.** There is no `resolve.alias` in `vite.config.js`. All imports use relative paths. Do not assume `@` works — it will break.

### Imports and auto-imports
**No `unplugin-auto-import` or `unplugin-vue-components`.** Every component and function must be explicitly imported. Vue 3 Composition API functions (`ref`, `computed`, etc.) must be imported manually.

### API client (`src/router/axios.js`)
- Named export `http` — import as `import { http } from "../router/axios.js"`.
- **Request interceptor:** sets `contentStore.loading = true`, attaches `Authorization: Bearer <token>`.
- **Response interceptor:** stores returned `response.data.token` in `authStore` + `localStorage` if present.
- **Error interceptor:** 401 → logout; 403/429/500 → `dialogStore.showNotification("fail", "...")`. All messages are hardcoded **Traditional Chinese**.
- `mapStore.js` fetches local GeoJSON and GeoServer WFS using bare `axios` (not `http`) — those calls bypass all interceptors.

### Pinia stores
All use **Options API style** (`defineStore(id, { state, getters, actions })`), except `chatStore` (Setup function style).

| Store | Owns |
|---|---|
| `authStore` | JWT token, user object, `isMobileDevice`/`isNarrowDevice`, `currentPath` |
| `contentStore` | Dashboards, components, map layers, `loading`/`error` flags (toggled by axios interceptors), `dashboardTimeRange`, favorites |
| `mapStore` | Raw Mapbox `Map` instance (`markRaw`), deck.gl overlay, layer configs, map state, user location |
| `dialogStore` | 28 named boolean dialog flags, notification state, `showDialog()` / `showNotification()` |
| `adminStore` | Admin CRUD state for dashboards, components, issues, disasters, users, contributors |
| `chatStore` | AI chat messages (persisted to `sessionStorage`), recommended components |

- **`contentStore.loading`** is the global loading flag — toggled by the axios interceptor, not by individual components.
- **Pinia debounce plugin** in `main.js` wraps actions listed in a store's `debounce` option with `lodash.debounce`. Only `contentStore.favoriteComponent` and `unfavoriteComponent` (500ms) use it.
- **`authStore.initialChecks()`** is called from `App.vue` (not `main.js`) — reads token from `localStorage`, validates against `/user/me`, determines device type.

### Routing (`src/router/index.js`)
Five sequential `beforeEach` guards (run in order):
1. Sets `authStore.currentPath`
2. Mobile redirect — narrow devices only allowed on: `dashboard`, `component-info`, `callback`, `embed`, `mapview`
3. Auth guard — admin routes require `is_admin && token`; has a 200ms timeout fallback for async token hydration
4. Content loading — calls `contentStore.setRouteParams()`, `contentStore.setDashboards()`, `mapStore.clearEntireMap()`
5. Admin data loading — calls `adminStore.setRouteParams()` for `/admin/dashboard`

Dashboard and map views use **query params**: `?index=<dashboardIndex>&city=<cityKey>`.

### Component architecture

```
src/
  components/
    charts/           # HistoryChart.vue (ApexCharts)
    dialogs/          # All app dialogs + admin/ sub-dir
    map/              # MapContainer, MapPopup
    utilities/bars/   # NavBar, SideBar, AdminSideBar, TimeRangePicker, etc.
  dashboardComponent/ # Core data-viz design system (separate concern)
    DashboardComponent.vue   # Main card component (~961 lines)
    components/       # 23 chart type components
    utilities/        # *.ts TypeScript helpers (only .ts files in project)
    styles/
  views/              # Page-level views
    admin/            # 6 admin pages (lazy-loaded)
```

- **`dashboardComponent/` is the core visualization system.** `DashboardComponent.vue` accepts a `mode` prop: `"default"`, `"large"`, `"map"`, `"half"`, `"halfmap"`, `"preview"`.
- **`dashboardComponent/utilities/*.ts` are TypeScript** — the only `.ts` files in the project. They are NOT covered by ESLint (ESLint only targets `*.js` and `*.vue`).
- **Dialog pattern:** All dialogs are boolean-flagged via `dialogStore.dialogs.<name>`. `DialogContainer.vue` is the single mount point. Use `dialogStore.showDialog("name")` / `dialogStore.hideAllDialogs()`.

### Styling
- **Dark theme.** CSS custom properties on `:root` — no CSS framework, no SCSS in global styles.
- SCSS (`sass`) is used inside component `<style lang="scss">` blocks only.
- Key CSS vars: `--color-background: #090909`, `--color-highlight: #5a9cf8`, `--color-component-background: #282a2c`.
- Typography: Chinese-first font stack `"微軟正黑體", "Microsoft JhengHei", ...`. All UI text is **hardcoded Traditional Chinese** — no i18n library.
- Icons: `material-icons` npm package loaded as an icon font. Usage: `<span class="material-icons-round">icon_name</span>`. Not SVG components.
- Global `overflow: hidden` on `*`; scrollbars hidden globally via `::-webkit-scrollbar { width: 0 }`.

### Notable third-party libraries
| Library | Usage |
|---|---|
| `mapbox-gl` ^3 | Main map, raw instance in `mapStore.map` |
| `@deck.gl/*` ^9 | Arc layers on Mapbox via `MapboxOverlay` |
| `three` ^0.163 + `threebox-plugin` | 3D MRT train animation layers |
| `vue3-apexcharts` | Registered globally as `<apexchart>` via `app.use(VueApexCharts)` |
| `@turf/turf` | Geospatial math in `mapStore` |
| `hls.js` | HLS video streams for CCTV map layers |
| `dayjs` | Date utilities |
| `@vueuse/core` | Composition utilities |

### Other non-obvious facts
- **`gtag` is a browser global** — Google Analytics 4 is loaded externally, not via npm. Declare `/* global gtag */` in files that use it or ESLint will error.
- **Static geo assets live in `public/`**, not `src/assets/`: GeoJSON at `/mapData/*.geojson`, map images at `/images/map/*.png`, 3D GLB models at `/images/map/mrt_car_*.glb`. These are fetched at runtime via `fetch()` or bare `axios`.
- **Multi-city support:** city context passed as `?city=` query param. Most components have a `city` property. CSS vars `--color-taipei` and `--color-metrotaipei` tag content by city.
- **Layer ID format:** `{component_index}-{type}-{city}` (e.g. `bike_stop-circle-taipei`).

---

## Backend (`Taipei-City-Dashboard-BE/`)

**Go 1.24**. Entry: `main.go` → `cmd/root.go` (Cobra CLI).

```bash
go mod tidy
go run main.go              # run dev server
go build -v ./...           # build binary
go run main.go migrateDB    # run DB schema migrations (not auto-run on startup)
go run main.go initDashboard # seed dashboard data
go test ./...               # run all tests
go test ./app/controllers/ -run TestFunctionName   # run single test
```

### Architecture
- **Two separate PostgreSQL databases**: `dashboard` (chart/component data) and `dashboardmanager` (user/auth). Both must be running.
- All env vars are loaded at package init in `global/global.go` — no runtime config file parsing.
- Redis for caching and rate limiting.
- Qdrant vector DB for AI semantic search.
- **ONNX model not in repo** — must be placed at `LM_MODEL_PATH` (default `/opt/lm_model/onnx-e5/`) separately. Backend requires this for AI chat features.
- JWT auth applied globally; TaipeiPass/ISSO OAuth2 for city gov SSO.
- `go run main.go migrateDB` is explicit — schema migrations do not auto-run on server start.

### CI gates (backend)
Only `go build -v ./...` is checked in CI. No linter (golangci-lint) is configured.

---

## Full-stack local dev via Docker Compose

```bash
# One-time: create the external bridge network (compose will fail without it)
docker network create --driver=bridge --subnet=192.168.128.0/24 --gateway=192.168.128.1 br_dashboard

cp docker/.env.template docker/.env   # fill in secrets

docker compose -f docker/docker-compose-db.yaml up -d     # postgres x2, redis, qdrant
docker compose -f docker/docker-compose.yaml up -d        # nginx, FE, BE
docker compose -f docker/docker-compose-init.yaml up      # one-time DB seed
```

The `docker-compose.yaml` references a local image `dashboard-be-dev:latest` that must be built locally first.

---

## Data Engineering (`Taipei-City-Dashboard-DE/`)

Managed entirely via Docker Compose (Airflow scheduler/worker/webserver/flower). No standalone Python CLI commands. Queue routing (`realtime`, `default`, `heavy`) is assigned automatically in `dags/operators/common_pipeline.py` based on `schedule_interval`.

---

## Branch and deploy model

| Branch | CI action |
|---|---|
| any (PR) | Frontend: prettier check + build |
| `main` | Backend: `go build` |
| `sit` | Build & push Docker images → deploy to SIT (Azure AKS) |
| `pre-develop` / `develop` | Build & push Docker images → deploy to prod (Azure AKS) |

Image tag format: `{branch}-{8-char-sha}`. Helm values: `helm-chart/values-sit.yaml` or `values-prod.yaml`.
# Repository Guidelines

## Project Structure & Module Organization

`Taipei-City-Dashboard-FE/` is the Vue 3/Vite frontend; use `src/` for app code, `public/` for static files, and `public/mapData/` for GeoJSON. `Taipei-City-Dashboard-BE/` is the Go API under `app/controllers`, `app/models`, `app/services`, `app/routes`, and `app/middleware`. `Taipei-City-Dashboard-DE/` contains Airflow DAGs in `dags/`, shared code in `dags/operators` and `dags/utils`, and tests in `dags/test`. Root `docker/`, `helm-chart/`, `sql/`, and `db-sample-data/` hold local services, deployment manifests, schemas, and seed data.

## Build, Test, and Development Commands

- Frontend: `cd Taipei-City-Dashboard-FE && npm ci` installs locked dependencies.
- Frontend dev: `npm run dev` starts Vite; `npm run preview` serves the production build.
- Frontend validation: `npm run lint` runs ESLint with auto-fix; `npm run build` lints and builds.
- Backend: `cd Taipei-City-Dashboard-BE && go run main.go` runs the API; `go build -v ./...` matches CI.
- Backend tests: `go test ./...` should pass when Go tests are added or changed.
- Data engineering tests: from `Taipei-City-Dashboard-DE/`, run `python -m pytest dags/test`; run `python -m pytest cicd/utils` only with Google Cloud test configuration.
- Full stack: create `docker/.env` from `docker/.env.template`, then follow `DOCKER.md`.

## Coding Style & Naming Conventions

Frontend code uses ES modules, Vue single-file components, and tab indentation enforced by `eslint.config.js`. Avoid `console.log`; `console.warn` and `console.error` are allowed. Keep Vue components in PascalCase, stores in `src/store`, routes in `src/router`, and helpers in `src/assets/utilityFunctions`. Go code must be `gofmt`/`go vet` clean, with lowercase package names and handlers split across controllers/services/models. Airflow DAG folders pair one Python DAG file with `job_config.json`.

## Testing Guidelines

Place Python tests as `test_*.py` near existing areas (`dags/test`, `cicd/utils`). Prefer small tests around transformations, SQL generation, queue routing, and DAG utilities. Frontend has no unit test script; validate UI changes with `npm run build` and browser checks. Backend changes should include focused Go tests where logic is isolated.

## Commit & Pull Request Guidelines

Recent history uses short, scoped summaries, often in Traditional Chinese, naming the affected ETL or module. Keep commits focused and put the changed area first. PRs must start from an issue, per `.github/PULL_REQUEST_TEMPLATE.md`; include the issue link, summary, type checkbox, tests run, and screenshots for visible frontend changes. Run relevant checks before review.

## Security & Configuration Tips

Never commit real secrets. Local values belong in ignored files such as `docker/.env` and `Taipei-City-Dashboard-FE/.env`. Keep Mapbox tokens, JWT secrets, salts, database passwords, and Qdrant keys out of source and PR screenshots.
