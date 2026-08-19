# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A Go + React web app with cookie-session auth as its foundation, plus an in-progress matchmaking/lobby system. The backend serves the built frontend directly — the whole thing ships as one binary/one Docker image. Requires Go 1.26+, Node 24+, Docker + Docker Compose.

## Commands

### Backend (`be/`, Go module, `go 1.26`)
- Build: `cd be && go build ./...`
- Test: `cd be && go test ./...` (single test: `go test ./persist/... -run TestCreateAndFetchUser -v`)
- Lint: `cd be && golangci-lint run ./...` (config: `be/.golangci.yml`)
- Run locally: `make run_be` (= `go run main.go http --env local`, reads `be/infra/envs/local/.env`)
- Migrations: `make run_migrations ENV=local` / `make run_down_migrations ENV=local`
- Regenerate `fe/src/types.ts` from Go API structs: `make generate_types`

### Frontend (`fe/`, Vite + React 19 + TS)
- Dev server: `make run_fe` (= `npm run dev`, proxies `/api` to `:8091`, see `fe/vite.config.ts`)
- Build (includes typecheck): `npm run build` — runs `tsc --noEmit && vite build`
- Lint: `npm run lint` (ESLint flat config: `fe/eslint.config.js`)
- Typecheck only: `npm run typecheck`
- e2e tests: `cd fe && npm test` (Playwright; needs the full stack running, see below)

### Full stack (closest to production, what CI runs)
`docker compose up --build` — builds/starts `db` (MySQL) → `db-seeder` (migrates + seeds fixture data, one-shot) → `app` (combined Go+React binary) on `:8091`.

### Fast local iteration
`make run_db` (db only) → `make run_migrations ENV=local` → `make run_be` → `make run_fe`. Open `:5173`. Seeded test login: `test` / `test`.

## Architecture

**One binary, three subcommands.** `be/main.go` → `cmd.Execute()` (cobra). `be http` runs the server, `be migrations` runs schema migrations as a separate one-shot process — deliberately never on app boot; matches how `compose.yaml`'s `db-seeder` gates `app` startup — and `be jobs --job generate-types` regenerates `fe/src/types.ts` from Go API structs (`be/api/*.go`) via `tkrajina/typescriptify-golang-structs`. **`fe/src/types.ts` is generated — don't hand-edit it.**

**Migrations/seed SQL is embedded in the binary** (`be/db/embed.go`, `go:embed`), not read from disk at runtime, so the migration runner behaves identically regardless of working directory or deploy layout. `--seed true` is refused for any `--env` other than `local`/`test` (seed data is fake fixture data; guarded in `be/cmd/migrations.go`).

**Auth**: bcrypt password hashes + `gorilla/sessions` cookie sessions. `handlers.AuthMiddleware` (`be/handlers/handlers.go`) gates every `/api/*` route except login/create. Frontend's `GlobalDataContext` (`fe/src/GlobalDataContext.tsx`) fetches `/api/data` once on mount and redirects based on the response (401 → `/login`).

**Persistence**: raw `database/sql` + `go-sql-driver/mysql`, no ORM (`be/persist/`). `persist.NewDB(env)` only special-cases `env == "local"` (loads `be/infra/envs/local/.env`); any other env (`test`, `prod`, ...) reads `DB_USERNAME`/`DB_PASSWORD`/`DB_HOST`/`DB_NAME` straight from the process environment.

**Matchmaking/lobby** (`be/matchmaking/`) is currently pure in-memory state on the handler struct — not persisted, won't survive a restart or multiple instances, despite a `game_matches` migration already existing for it. This is known-incomplete, not a bug to silently "fix" by adding persistence unasked.

**Testing philosophy**: `be/persist/users_test.go` is a real integration test against MySQL (via `compose.yaml`'s `db` service, or CI's service container), not a mocked driver — a mock only proves the code calls `Exec`/`Query` with a matching string, not that the SQL is correct against a real engine. It skips (`t.Skip`, not a failure) when no DB is reachable, so `go test ./...` still works standalone.

**CI** (`.github/workflows/`): `playwright.yml` (e2e, full docker-compose stack), `lint-be.yml` (golangci-lint), `lint-fe.yml` (eslint + tsc + prettier --check), `test-be.yml` (go test against a real `mysql:8.0` service container).

## Known gotchas

- `typescript` is pinned to `6.0.3`, not latest — `typescript-eslint`'s peer range is `>=4.8.4 <6.1.0` and doesn't yet support the newer TS native compiler. Don't bump it past 6.x without checking `typescript-eslint` compatibility first.
- A native `mysql` install commonly squats on host port `3306` on dev machines; `compose.yaml`'s `db` service publishes `3306:3306`, so this can collide locally (remap the port for local testing rather than touching the committed file).
