generate_types:
	@cd be && go run main.go generate-types
	@mv ./be/types.ts ./fe/src/types.ts

run_db:
	@docker compose up db

run_be:
	@cd be && go run main.go http --env local

run_fe:
	@cd fe && npm run dev

run_migrations:
	@cd be && go run main.go migrations --env ${ENV} --direction up --seed true

run_down_migrations:
	@cd be && go run main.go migrations --env ${ENV} --direction down

# Prod-specific - not folded into run_migrations/ENV=prod above, since that
# target always passes --seed true (fine for local/test, refused outright
# for prod - seed data is fake fixture data). Loads
# be/infra/envs/prod/.env.local (real credentials, never committed - see
# be/.gitignore) into the recipe's own subshell via `set -a` so DB_HOST/etc.
# actually reach the go process, rather than just sitting unused in a file.
run_prod_migrations:
	@cd be && set -a && . infra/envs/prod/.env.local && set +a && go run main.go migrations --env prod --direction up

# make run_prod_process_set NAME=<set-slug> [ARGS=--skip-images]
run_prod_process_set:
	@cd be && set -a && . infra/envs/prod/.env.local && set +a && go run main.go process-set --env prod --name ${NAME} --refresh ${ARGS}
