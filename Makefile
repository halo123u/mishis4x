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

# make run_prod_set_price_sources NAME=<set-slug>
run_prod_set_price_sources:
	@cd be && set -a && . infra/envs/prod/.env.local && set +a && go run main.go set-price-sources --env prod --name ${NAME}

# make run_prod_model_import ARGS="--char 002406 --set-name '<real set name>' --card BRD/W139-001S --card BRD/W139-003S"
# ARGS is the whole flag string, not just --char, since --card/--set-name
# are optional (see be/cmd/model_import.go's own doc comment) and there's
# no single positional arg this target could sensibly default to instead.
run_prod_model_import:
	@cd be && set -a && . infra/envs/prod/.env.local && set +a && go run main.go model-import --env prod ${ARGS}

# make run_prod_sync_prices - manual one-shot (see be/cmd/sync_prices.go);
# there's no background job running this automatically yet, so re-run
# periodically until that exists.
run_prod_sync_prices:
	@cd be && set -a && . infra/envs/prod/.env.local && set +a && go run main.go sync-prices --env prod
