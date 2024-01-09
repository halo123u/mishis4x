generate_types:
	@echo "Generating types..."
	@cd be && go run main.go jobs --job generate-types
	@mv ./be/types.ts ./fe/src/types.ts


	
