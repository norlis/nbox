
# --- Configuración y Variables ---
APP_IMPORT_PATH := $(shell go list -m)
ALL_PKGS := $(sort $(shell go list ./...))

# Variables para inyectar en el build
GIT_SHA := $(shell git rev-parse --short HEAD)

DATE := $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')

LDFLAGS := -ldflags "-s -w \
	-X $(APP_IMPORT_PATH)/internal/version.GitHash=$(GIT_SHA) \
	-X $(APP_IMPORT_PATH)/internal/version.Date=$(DATE)"

# --- Herramientas y Módulos ---
TOOLS_BIN_DIR := $(abspath ./bin)
TOOLS_MOD_DIR := $(abspath ./tools)

export PATH := $(TOOLS_BIN_DIR):$(PATH)

# --- Comandos Base ---
GOBUILD := GO111MODULE=on CGO_ENABLED=0 go build -trimpath

# ====================================================================================
# Comandos Públicos
# ====================================================================================
.PHONY: help all build clean test lint format check-format tools

help:
	@echo "Uso: make [comando]"
	@echo ""
	@echo "## --- Ciclo de Vida del Build ---"
	@echo "  build            Construye los binarios para todas las plataformas."
	@echo "  <arch>-build             Compila los 3 binarios para <arch> (amd64|arm64); GOOS autodetectado."
	@echo "  <arch>-build-nbox        Compila microservice + cli para <arch> (amd64|arm64)."
	@echo "  <arch>-build-entrypushd  Compila entrypushd para <arch> (amd64|arm64)."
	@echo "  clean            Elimina la carpeta de build."
	@echo ""
	@echo "## --- Calidad de Código ---"
	@echo "  lint             Ejecuta todos los linters (golangci-lint y staticcheck)."
	@echo "  format           Formatea automáticamente el código con goimports."
	@echo "  check-format     Verifica el formato sin modificar archivos (ideal para CI)."
	@echo "  test             Ejecuta las pruebas unitarias."
	@echo "  test-sonar       Ejecuta pruebas generando reportes para SonarQube."
	@echo ""
	@echo "## --- Gestión de Dependencias ---"
	@echo "  tools            Instala/actualiza las herramientas de desarrollo en ./bin."
	@echo "  mod-tidy         Ejecuta 'go mod tidy' en el módulo principal."
	@echo "  mod-vendor       Ejecuta 'go mod vendor'."


all: check-format lint test build


## ----------------------------------------
## Gestión de Herramientas
## ----------------------------------------
tools: $(TOOLS_BIN_DIR)

$(TOOLS_BIN_DIR): $(TOOLS_MOD_DIR)/tools.go $(TOOLS_MOD_DIR)/go.mod
	@echo "==> Instalando herramientas desde tools/go.mod..."
	@mkdir -p $(TOOLS_BIN_DIR)
	@cd $(TOOLS_MOD_DIR) && go mod tidy
	# Usamos go list para obtener las herramientas de forma robusta
	@cd $(TOOLS_MOD_DIR) && \
		go list -e -f '{{range .Imports}}{{.}} {{end}}' -tags=tools tools.go | \
		xargs -n1 env GOBIN=$(TOOLS_BIN_DIR) go install -v
	@touch $(TOOLS_BIN_DIR)
	@echo "==> Herramientas actualizadas."

.PHONY: tools-force
tools-force:
	@rm -rf $(TOOLS_BIN_DIR)
	@$(MAKE) tools


## ----------------------------------------
## Calidad de Código
## ----------------------------------------
lint: tools
	@echo "==> Ejecutando golangci-lint..."
	@$(TOOLS_BIN_DIR)/golangci-lint run --fix

vulncheck: tools
	@echo "==> Escaneando vulnerabilidades (govulncheck)..."
	@$(TOOLS_BIN_DIR)/govulncheck ./...

format: tools
	@echo "==> Formateando código..."
	@$(TOOLS_BIN_DIR)/gofumpt -l -w .

check-format: tools
	@echo "==> Verificando formato..."
	@if [ -n "$$($(TOOLS_BIN_DIR)/gofumpt -l .)" ]; then \
		echo "ERROR: El código no está formateado con gofumpt."; \
		exit 1; \
	fi


## ----------------------------------------
## Pruebas y SonarQube
## ----------------------------------------
test:
	@echo "==> Ejecutando pruebas unitarias..."
	@go test ./... --cover
	@#go test -race ./...  --cover

test-sonar:
	@echo "==> Generando reportes para SonarQube..."
	@go test -covermode=atomic -coverprofile=coverage.out ./...
	@go test -json ./... > report.json

run-local-sonar:
	docker run -d --name sonarqube -p 9000:9000 sonarqube


## ----------------------------------------
## Build y Clean
## ----------------------------------------
build: clean
	@echo "==> Construyendo binarios..."
	GOOS=darwin  GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o ./build/darwin/amd64/microservice ./cmd/nbox
	GOOS=darwin  GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o ./build/darwin/amd64/entrypushd  ./cmd/entrypushd
	GOOS=darwin  GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o ./build/darwin/amd64/cli    ./cmd/cli
	GOOS=darwin  GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o ./build/darwin/arm64/microservice ./cmd/nbox
	GOOS=darwin  GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o ./build/darwin/arm64/entrypushd  ./cmd/entrypushd
	GOOS=darwin  GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o ./build/darwin/arm64/cli    ./cmd/cli
	GOOS=linux   GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o ./build/linux/amd64/microservice  ./cmd/nbox
	GOOS=linux   GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o ./build/linux/amd64/entrypushd   ./cmd/entrypushd
	GOOS=linux   GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o ./build/linux/amd64/cli     ./cmd/cli
	GOOS=linux   GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o ./build/linux/arm64/microservice  ./cmd/nbox
	GOOS=linux   GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o ./build/linux/arm64/entrypushd   ./cmd/entrypushd
	GOOS=linux   GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o ./build/linux/arm64/cli     ./cmd/cli
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o ./build/windows/amd64/microservice ./cmd/nbox
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o ./build/windows/amd64/cli     ./cmd/cli

# OS destino: autodetecta el del host (darwin en Mac, linux en el contenedor de build).
# Override para cross-compile: GOOS=linux make arm64-build
GOOS ?= $(shell go env GOOS)

# make <arch>-build-nbox        → microservice + cli
# make <arch>-build-entrypushd  → entrypushd
%-build-nbox:
	GOOS=$(GOOS) GOARCH=$* $(GOBUILD) $(LDFLAGS) -o ./build/$(GOOS)/$*/microservice ./cmd/nbox
	GOOS=$(GOOS) GOARCH=$* $(GOBUILD) $(LDFLAGS) -o ./build/$(GOOS)/$*/cli          ./cmd/cli

%-build-entrypushd:
	GOOS=$(GOOS) GOARCH=$* $(GOBUILD) $(LDFLAGS) -o ./build/$(GOOS)/$*/entrypushd   ./cmd/entrypushd

# make <arch>-build             → microservice + cli + entrypushd
%-build: %-build-nbox %-build-entrypushd
	@:

clean:
	@echo "==> Limpiando builds anteriores..."
	@rm -rf ./build


## ----------------------------------------
## Gestión de Módulos
## ----------------------------------------
mod-tidy:
	go mod tidy

mod-vendor:
	go mod vendor


## ----------------------------------------
## otras
## ----------------------------------------
.PHONY: docs
docs:
	$(TOOLS_BIN_DIR)/swag init -g internal/transport/http/server.go --parseDependency


## ----------------------------------------
## Proto codegen
## ----------------------------------------
.PHONY: proto-gen
proto-gen:
	@echo "==> Generando código gRPC desde proto..."
	@protoc \
	  --go_out=. \
	  --go_opt=module=nbox \
	  --go-grpc_out=. \
	  --go-grpc_opt=module=nbox \
	  proto/kvstream.proto
	@echo "==> Generado en gen/stream/v1/"
