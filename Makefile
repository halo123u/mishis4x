generate_types:
	@cd be && go run main.go jobs --job generate-types
	@mv ./be/types.ts ./fe/src/types.ts
run_be:
	@cd be && docker build -t mishis4x-be .
	@docker run -p 8091:8091 -e DB_USERNAME=$(DB_USERNAME) -e DB_HOST=$(DB_HOST) -e DB_PASSWORD=$(DB_PASSWORD) -e  DB_NAME=$(DB_NAME) mishis4x-be
run_migrations:
	@cd be && go run main.go migrations --env ${ENV} --direction up --seed true
run_down_migrations:
	@cd be && go run main.go migrations --env ${ENV} --direction down