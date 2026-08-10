# Define variables
API_SPEC=api/api.yaml
API_GEN_CONFIG=api/api.codegen.yaml
API_GEN_OUTPUT=api/api.gen.go

# Go commands
GO_RUN=go run main.go
GO_TIDY=go mod tidy

# OpenAPI code generation
.PHONY: api-generate
api-generate:
	@echo "🚀 Generating API code..."
	oapi-codegen -config $(API_GEN_CONFIG) $(API_SPEC)

# Run the Fiber server
.PHONY: run
run: api-generate
	@echo "🚀 Starting Fiber server..."
	$(GO_RUN)

# Install dependencies
.PHONY: install
install:
	@echo "📦 Installing dependencies..."
	go install github.com/deepmap/oapi-codegen/cmd/oapi-codegen@latest
	go mod tidy

# Clean generated files
.PHONY: clean
clean:
	@echo "🧹 Cleaning generated files..."
	rm -f $(API_GEN_OUTPUT)

# Full setup (install dependencies, generate API, run server)
.PHONY: setup
setup: install api-generate run
