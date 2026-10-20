# Define variables
API_SPEC=api/api.yaml
API_GEN_CONFIG=api/api.codegen.yaml
API_GEN_OUTPUT=api/api.gen.go
OAPI_CODEGEN_VERSION=v1.16.3

# Go commands
GO_RUN=go run main.go
GO_TIDY=go mod tidy

# OpenAPI code generation
.PHONY: api-generate
api-generate:
	@echo "🚀 Generating API code..."
	go run github.com/deepmap/oapi-codegen/cmd/oapi-codegen@$(OAPI_CODEGEN_VERSION) -config $(API_GEN_CONFIG) $(API_SPEC)

# Run the Fiber server
.PHONY: run
run: api-generate
	@echo "🚀 Starting Fiber server..."
	$(GO_RUN)

# Install dependencies
.PHONY: install
install:
	@echo "📦 Installing dependencies..."
	go install github.com/deepmap/oapi-codegen/cmd/oapi-codegen@$(OAPI_CODEGEN_VERSION)
	go mod tidy

# Clean generated files
.PHONY: clean
clean:
	@echo "🧹 Cleaning generated files..."
	rm -f $(API_GEN_OUTPUT)

# Full setup (install dependencies, generate API, run server)
.PHONY: setup
setup: install api-generate run

.PHONY: commerce-onboard-bing-chun
commerce-onboard-bing-chun:
	go run ./cmd/onboard-commerce -config config/merchants/bing-chun-nigeria.json

.PHONY: commerce-e2e
commerce-e2e:
	./scripts/run-commerce-e2e.sh
