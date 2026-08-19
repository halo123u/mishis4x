# mishis4x

A Go + React web app with cookie-session auth as its foundation, plus an
in-progress matchmaking/lobby system. The backend serves the built
frontend directly, so the whole thing ships as one binary/one image.

- `be/` — Go backend. A single `cobra` CLI (`be http`, `be migrations`,
  `be jobs`) built from `be/main.go`.
- `fe/` — React + TypeScript frontend (Vite).
- `compose.yaml` — local/CI orchestration (MySQL + a one-shot migration
  job + the app).

## Requirements

- Go 1.21+
- Node 20+
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

`SESSION_SECRET` (used by the `http` subcommand only, to sign the login
session cookie) is required for every env, including local — the app
refuses to start without it rather than falling back to a hardcoded key.
The local `.env`/`.env.example` value is a placeholder that's explicitly
rejected outside `local`/`test`; a real deployment needs its own, generated
with `openssl rand -hex 32` and set as a real secret, never committed.

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
  of where or how it's invoked.
