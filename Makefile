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
