# mishis4x

A Go + React web app with a hardened account system (signup, login,
logout, change-password, rate-limited, fully revocable server-side
sessions) as its foundation, plus an in-progress matchmaking/lobby system
that hasn't been built on top of it yet. The backend serves the built
frontend directly, so the whole thing ships as one binary/one image.

- `be/` — Go backend. A single `cobra` CLI (`be http`, `be migrations`,
  `be jobs`) built from `be/main.go`.
- `fe/` — React + TypeScript frontend (Vite).
- `compose.yaml` — local/CI orchestration (MySQL + a one-shot migration
  job + the app).

## Requirements

- Go 1.26+
- Node 24+
- Docker + Docker Compose

## Running it locally

There are two ways to run this, for two different purposes.

### 1. Full stack — closest to production, what CI runs

```
docker compose up --build
```

This builds and starts, in order: `db` (MySQL), `db-seeder` (runs
migrations + seeds fixture data once, then exits), then `app` (the
combined Go+React binary) on **http://localhost:8091**.

Use this when you want to test the app as it'll actually run in
production, or as the target for the Playwright e2e suite (`fe/tests`).

Tear down with `docker compose down` (add `-v` to also wipe the MySQL
data volume and start fully fresh next time).

### 2. Fast iteration — hot reload, no rebuild loop

Run each piece natively so changes reload instantly instead of round-
tripping through a Docker build.

```
make run_db          # starts just MySQL via compose
make run_migrations ENV=local   # migrate + seed it (first time only)
make run_be           # go run main.go http --env local
make run_fe           # vite dev server
```

Open **http://localhost:5173**. The Vite dev server proxies `/api/*` to
the Go server on `:8091` (see `fe/vite.config.ts`), so both need to be
running.

Log in with the seeded test user: `test` / `test` (see
`be/db/seeds/001_users_seed.sql`).

## Environment variables

`be/infra/envs/local/.env` is loaded automatically whenever a command
runs with `--env local` (the default for every `be` subcommand). See
`be/infra/envs/local/.env.example` for what's required. For any other
`--env` (`test`, `prod`, ...), the same variables must be set as real
environment variables instead — `NewDB` only reads a `.env` file for
`local`.

Sessions don't need a signing secret: login/signup issue a random opaque
token (`persist.NewSessionToken`, 256 bits from `crypto/rand`) stored server-
side in the `sessions` table, and the cookie only ever carries that token.
There's nothing to sign, encrypt, or leak — the token's own randomness plus
the server-side lookup is what authenticates a request, and logout deletes
the row outright instead of just clearing a client-side cookie.

## Common tasks

- Regenerate `fe/src/types.ts` from the Go API structs:
  `make generate_types`
- Run e2e tests (needs the full stack running — see above):
  `cd fe && npm test`
- Roll back migrations: `make run_down_migrations ENV=local`

## Notes

- `--seed true` is refused for any `--env` other than `local`/`test` —
  seed data is fake fixture data and should never land in a real
  database.
- Migration/seed SQL is embedded into the `be` binary at build time
  (`be/db/embed.go`), so the migration runner works the same regardless
  of where or how it's invoked. Both are tracked in a `schema_migrations`
  table, so re-running them against an already-migrated database (e.g.
  a normal restart against `compose.yaml`'s persisted volume) skips
  what's already applied instead of erroring.
- Login is rate-limited per username: 5 failed attempts within 15
  minutes triggers a 5-minute lockout (`be/handlers/loginLimiter.go`).
- Sessions are server-side — the cookie only ever carries an opaque
  random token, looked up against a `sessions` table. Logout and
  changing your password both actually delete session rows (revoked
  for real), not just clear a client-side cookie.
- `/healthcheck` verifies real DB connectivity (503 if unreachable),
  not just that the process is alive.
- The HTTP port is configurable via the `PORT` env var (defaults to
  `8091`) — most hosting platforms inject this rather than letting you
  pick a fixed port.
