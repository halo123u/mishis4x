# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A Go + React web app with a hardened account system (signup, login, logout, change-password, rate-limited and fully revocable server-side sessions) as its foundation, plus an in-progress matchmaking/lobby system that hasn't been built on top of that foundation yet. The backend serves the built frontend directly — the whole thing ships as one binary/one Docker image. Requires Go 1.26+, Node 24+, Docker + Docker Compose.

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
`docker compose up --build` — builds/starts `db` (MySQL) → `db-seeder` (migrates + seeds fixture data, one-shot) → `app` (combined Go+React binary) on `:8091`. `db` has a persistent volume, and migrations/seeds are tracked (see Architecture below), so a plain restart (no `-v`) is safe and fast — it skips everything already applied instead of re-running or erroring.

### Fast local iteration
`make run_db` (db only) → `make run_migrations ENV=local` → `make run_be` → `make run_fe`. Open `:5173`. Seeded test login: `test` / `test`.

## Architecture

**One binary, three subcommands.** `be/main.go` → `cmd.Execute()` (cobra). `be http` runs the server, `be migrations` runs schema migrations as a separate one-shot process — deliberately never on app boot; matches how `compose.yaml`'s `db-seeder` gates `app` startup — and `be jobs --job generate-types` regenerates `fe/src/types.ts` from Go API structs (`be/api/*.go`) via `tkrajina/typescriptify-golang-structs`. **`fe/src/types.ts` is generated — don't hand-edit it.**

**Migrations/seed SQL is embedded in the binary** (`be/db/embed.go`, `go:embed`), not read from disk at runtime, so the migration runner behaves identically regardless of working directory or deploy layout. Both migrations and seeds are tracked in a `schema_migrations` table keyed by file path (`be/persist/migrations.go`), so re-running `up`/seed against an already-migrated DB skips whatever's already applied instead of erroring — this is what makes `compose.yaml`'s persistent `db` volume safe to restart against. `down` is all-or-nothing: it clears all tracking rather than mapping individual down-files back to the up-file they reverse. Execution order is plain lexical filename sort (not a parsed sequence number) — see Known Gotchas. `--seed true` is refused for any `--env` other than `local`/`test` (seed data is fake fixture data; guarded in `be/cmd/migrations.go`).

**Auth**: bcrypt password hashes + server-side sessions, not signed cookies. Login/signup issue a random opaque token (`persist.NewSessionToken`, 256 bits from `crypto/rand`) stored in a `sessions` table (`be/persist/sessions.go`); the cookie only ever carries that token — there's no signing secret, nothing to leak, the token's own entropy plus the server-side row lookup *is* the authentication check. `handlers.AuthMiddleware` (`be/handlers/handlers.go`) looks the token up per request and gates every `/api/*` route except login/create. Logout (`DeleteSession`) and changing your password (`POST /api/user/password`, which also calls `DeleteOtherSessions`) both actually delete rows — real revocation, not just a cleared client cookie. Login is rate-limited per username (`be/handlers/loginLimiter.go`: in-memory, 5 failed attempts / 15 min window / 5 min lockout — in-memory is deliberate given this runs as a single instance, same constraint as matchmaking below), and wrong-password vs. no-such-user return an identical error so the endpoint can't be used to enumerate usernames. Frontend's `GlobalDataContext` (`fe/src/GlobalDataContext.tsx`) fetches `/api/data` once on mount and redirects based on the response (401 → `/login`, but not if already on `/login` or `/sign-up`).

**Security headers**: `handlers.securityHeadersMiddleware` sets `X-Content-Type-Options`, `Referrer-Policy`, and a `Content-Security-Policy` on every response, plus HSTS once `SessionCookieConfig.Secure` is true (i.e. not `local`/`test`). The CSP is scoped to exactly what the frontend loads (self + Google Fonts, per `fe/index.html`) — if the frontend ever starts loading from somewhere new, update it there too or requests will silently fail rather than error loudly.

**Persistence**: raw `database/sql` + `go-sql-driver/mysql`, no ORM (`be/persist/`). `persist.NewDB(env)` only special-cases `env == "local"` (loads `be/infra/envs/local/.env`); any other env (`test`, `prod`, ...) reads `DB_USERNAME`/`DB_PASSWORD`/`DB_HOST`/`DB_NAME` straight from the process environment. `ParseTime: true` is required in the `mysql.Config` — see Known Gotchas.

**Frontend styling**: a small hand-rolled design system, not a component library. `fe/src/styles/{tokens,base,utilities}.css` define the palette (dark-mode-first; teal is the one primary accent for actions/links/focus, ember is reserved for logout/destructive actions only), a 4px spacing scale, and type scale (Big Shoulders Display / Inter / IBM Plex Mono via Google Fonts). Component-specific styling is a `*.module.css` next to the component (Vite supports this natively). `fe/src/components/ui/Button.tsx` is the one shared primitive (`primary`/`secondary`/`ghost`/`danger` variants) — use it instead of a raw `<button>`.

**Matchmaking/lobby** (`be/matchmaking/`) is currently pure in-memory state on the handler struct — not persisted, won't survive a restart or multiple instances, despite a `game_matches` migration already existing for it. The API routes (`/api/lobbies`, `/api/lobbies/create`) exist and work, but there's no frontend UI calling them yet. This is known-incomplete, not a bug to silently "fix" by adding persistence unasked.

**Collection tracker** (`sets`/`cards`/`owned_sets`/`owned_cards` tables, `GET /api/sets`, `GET /api/sets/{id}/cards`, `Home.tsx`'s "Card Manager" widget → `/collection`): `sets`/`cards` deliberately use `CHAR(36)` UUIDv7 primary keys instead of `AUTO_INCREMENT` (see issue #75) — `users.id` itself stays `INT AUTO_INCREMENT` (see #76 for the separate, deliberately-deferred task of migrating that too). **Every collection-tracker route is gated by `ownerOnlyMiddleware` (`handlers.Data.CollectionOwnerUserID`) on top of the normal `AuthMiddleware`** — being logged in isn't enough, it has to be that one specific `users.id`. This isn't a general role/admin system; it exists because eBay's API License Agreement requires eBay's express prior written consent for any "Public Display" of data pulled from their APIs, and this app has open signup — so "authenticated" alone doesn't establish "nobody but the developer sees eBay-sourced data." Set via `COLLECTION_OWNER_USER_ID` (unset/`0` fails closed — nobody passes, not everybody). Local dev and the `compose.yaml` stack both default this to `1`, the seeded `test`/`test` user's ID.

**Testing philosophy**: integration tests against real MySQL (via `compose.yaml`'s `db` service, or CI's service container), never a mocked driver — a mock only proves the code calls `Exec`/`Query` with a matching string, not that the SQL is correct against a real engine. `be/persist` and `be/handlers` each have their own `testDB(t)` helper that skips (`t.Skip`, not a failure) when no DB is reachable, so `go test ./...` still works standalone. `be/handlers/users_test.go` goes further: it spins up a real `httptest.Server` from `Data.NewRouter()` (a plain method, not tied to `InitializeHttpServer`'s signal-handling/shutdown logic) and drives it with a real `*http.Client` + cookie jar per simulated "device" — testing the actual routing/middleware stack (AuthMiddleware, rate limiting, session revocation), not handler functions called in isolation.

**CI** (`.github/workflows/`): `playwright.yml` (e2e, full docker-compose stack), `lint-be.yml` (golangci-lint), `lint-fe.yml` (eslint + tsc + prettier --check), `test-be.yml` (go test against a real `mysql:8.0` service container).

## Known gotchas

- `typescript` is pinned to `6.0.3`, not latest — `typescript-eslint`'s peer range is `>=4.8.4 <6.1.0` and doesn't yet support the newer TS native compiler. Don't bump it past 6.x without checking `typescript-eslint` compatibility first.
- A native `mysql` install commonly squats on host port `3306` on dev machines; `compose.yaml`'s `db` service publishes `3306:3306`, so this can collide locally (remap the port for local testing rather than touching the committed file).
- `go-sql-driver/mysql` needs `ParseTime: true` in its `mysql.Config` (set in `persist.NewDB`) or `DATETIME`/`TIMESTAMP` columns scan as raw `[]byte` instead of `time.Time` — easy to miss until something actually scans into a `time.Time` field (this bit `sessions.expires_at` once already; any test DB helper that opens its own connection instead of going through `persist.NewDB` needs this set too).
- Migration/seed execution order is just lexical filename sort (`fs.WalkDir` in `be/persist/migrations.go`), not a parsed sequence number — existing files rely on their date-prefix naming happening to sort correctly. A new migration's filename must sort after every existing one, or it'll try to run out of order.
- `httptest.Server.Client()` caches and returns the *same* `*http.Client` on repeated calls. Building a "second, independent" test client from it and only reassigning `.Jar` silently aliases the first client's cookies too. Always build a fresh `&http.Client{Jar: ...}` per simulated client in a multi-session test (see `be/handlers/users_test.go`'s `newClient`).
