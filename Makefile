# Tên của Docker image và container
IMAGE_NAME := go-scan-app
CONTAINER_NAME := go-scan-app-temp

# Tên của các binary sẽ được trích xuất
SCANNER_BIN := scanner
DELETER_BIN := deleter
REPORTER_BIN := reporter
REPORTER_OPT_BIN := reporter_opt
CHECKDUP_BIN := checkdup

# Các target mặc định và giả (phony targets)
.PHONY: all build-image create-container copy-scanner copy-deleter copy-reporter copy-reporter-opt remove-container extract-binaries clean build-local test

# Target mặc định: build image và trích xuất binary
all: build-image extract-binaries

# Target để build local binaries với optimizations
build-local:
	@echo "Building optimized local binaries..."
	@echo "Building scanner..."
	go build -tags scanner -trimpath -ldflags="-s -w" -o $(SCANNER_BIN) .
	@echo "Building deleter..."
	go build -tags deleter -trimpath -ldflags="-s -w" -o $(DELETER_BIN) .
	@echo "Building reporter..."
	go build -tags reporter -trimpath -ldflags="-s -w" -o $(REPORTER_BIN) .
	@echo "Building checkdup..."
	go build -tags checkdup -trimpath -ldflags="-s -w" -o $(CHECKDUP_BIN) .
	@echo "Building optimized reporter..."
	go build -tags reporter -trimpath -ldflags="-s -w" -o $(REPORTER_OPT_BIN) report_optimized.go
	@echo "Local build complete!"

# Target để chạy tests
test:
	@echo "Running tests..."
	go test -v ./...

# Target để build Docker image
build-image:
	@echo "Building Docker image $(IMAGE_NAME)..."
	docker build -t $(IMAGE_NAME) .

# Target để tạo container tạm thời
create-container:
	@echo "Creating temporary container $(CONTAINER_NAME)..."
	docker create --name $(CONTAINER_NAME) $(IMAGE_NAME) $(SCANNER_BIN)

# Target để copy binary scanner
copy-scanner:
	@echo "Copying $(SCANNER_BIN) from container..."
	docker cp $(CONTAINER_NAME):/$(SCANNER_BIN) ./$(SCANNER_BIN)

# Target để copy binary deleter
copy-deleter:
	@echo "Copying $(DELETER_BIN) from container..."
	docker cp $(CONTAINER_NAME):/$(DELETER_BIN) ./$(DELETER_BIN)

# Target để copy binary reporter
copy-reporter:
	@echo "Copying $(REPORTER_BIN) from container..."
	docker cp $(CONTAINER_NAME):/$(REPORTER_BIN) ./$(REPORTER_BIN)

# Target để copy optimized reporter (local only)
copy-reporter-opt: build-local
	@echo "Optimized reporter built locally as $(REPORTER_OPT_BIN)"

# Target để xóa container tạm thời
remove-container:
	@echo "Removing temporary container $(CONTAINER_NAME)..."
	docker rm $(CONTAINER_NAME)

# Target tổng hợp để trích xuất các binary
extract-binaries: create-container copy-scanner copy-deleter copy-reporter remove-container

# Target để dọn dẹp: xóa binary cục bộ, container và image Docker
clean:
	@echo "Cleaning up..."
	-docker rm $(CONTAINER_NAME) 2>/dev/null || true
	-docker rmi $(IMAGE_NAME) 2>/dev/null || true
	-rm -f $(SCANNER_BIN) $(DELETER_BIN) $(REPORTER_BIN) $(REPORTER_OPT_BIN)
	@echo "Cleanup complete."

# Target để cài đặt dependencies
deps:
	@echo "Installing dependencies..."
	go mod download
	go mod tidy

# Target để optimize dependencies
deps-optimize:
	@echo "Optimizing dependencies..."
	go mod tidy -go=1.24
	go mod verify

# Target để build với các flag tối ưu
build-release:
	@echo "Building release binaries with maximum optimizations..."
	@echo "Building scanner (release)..."
	go build -tags scanner -trimpath -ldflags="-s -w -extldflags '-static'" -o $(SCANNER_BIN)_release .
	@echo "Building deleter (release)..."
	go build -tags deleter -trimpath -ldflags="-s -w -extldflags '-static'" -o $(DELETER_BIN)_release .
	@echo "Building reporter (release)..."
	go build -tags reporter -trimpath -ldflags="-s -w -extldflags '-static'" -o $(REPORTER_BIN)_release .
	@echo "Building optimized reporter (release)..."
	go build -tags reporter -trimpath -ldflags="-s -w -extldflags '-static'" -o $(REPORTER_OPT_BIN)_release report_optimized.go
	@echo "Release build complete!"

# Target để verify các binary
verify: build-local
	@echo "Verifying binaries..."
	./$(SCANNER_BIN) --help >/dev/null 2>&1 || echo "Scanner binary verification failed"
	./$(DELETER_BIN) --help >/dev/null 2>&1 || echo "Deleter binary verification failed"
	./$(REPORTER_BIN) --help >/dev/null 2>&1 || echo "Reporter binary verification failed"
	./$(REPORTER_OPT_BIN) --help >/dev/null 2>&1 || echo "Optimized reporter binary verification failed"
	@echo "Binary verification complete!"

# Target hiển thị thông tin build
info:
	@echo "Build Information:"
	@echo "  Go Version: $(shell go version)"
	@echo "  OS/Arch: $(shell go env GOOS)/$(shell go env GOARCH)"
	@echo "  Module: $(shell head -1 go.mod)"
	@echo "  Git Commit: $(shell git rev-parse --short HEAD 2>/dev/null || echo 'N/A')"
	@echo "  Build Time: $(shell date -u '+%Y-%m-%d %H:%M:%S UTC')"

# =============================================================================
# QNAS BUILD TARGETS
# =============================================================================

.PHONY: qnap qnap-clean qnap-amd64 qnap-arm64 qnap-compose qnap-test qnap-package

# Build QNAS version (default AMD64)
qnap: build-local
	@echo "Building QNAS optimized version..."
	./build-qnap.sh

# Build QNAS with verbose output
qnap-verbose:
	@echo "Building QNAS optimized version (verbose)..."
	./build-qnap.sh --verbose

# Build QNAS with specific version
qnap-version:
	@if [ -z "$(VERSION)" ]; then echo "Usage: make qnap-version VERSION=1.0.0"; exit 1; fi
	./build-qnap.sh --version $(VERSION)

# Clean QNAS build artifacts
qnap-clean:
	@echo "Cleaning QNAS build artifacts..."
	./build-qnap.sh --clean
	rm -rf qnap-build qnap-output qnap-packages
	rm -f qnap-scanner-*.tar.gz

# Build QNAS AMD64 only
qnap-amd64:
	@echo "Building QNAS AMD64 binaries..."
	./build-qnap.sh --verbose

# Build QNAS ARM64 (if supported)
qnap-arm64:
	@echo "Building QNAS ARM64 binaries..."
	./build-qnap-compose.sh --arm64 --clean --verbose

# Build QNAS multi-architecture with Docker Compose
qnap-compose:
	@echo "Building QNAS multi-architecture binaries..."
	./build-qnap-compose.sh --verbose

# Build QNAS all architectures
qnap-all:
	@echo "Building QNAS for all architectures..."
	./build-qnap-compose.sh --arm64 --clean --verbose

# Test QNAS binaries
qnap-test: qnap
	@echo "Testing QNAS binaries..."
	./build-qnap.sh --skip-tests
	cd qnap-build && ./scanner --help && ./deleter --help && ./reporter --help

# Create QNAS deployment package
qnap-package: qnap
	@echo "Creating QNAS deployment package..."
	./build-qnap.sh --clean --verbose

# Build QNAS and push to registry
qnap-push:
	@if [ -z "$(REGISTRY)" ]; then echo "Usage: make qnap-push REGISTRY=your-registry.com"; exit 1; fi
	./build-qnap.sh --push --registry $(REGISTRY)

# Quick QNAS deployment simulation
qnap-deploy-test: qnap
	@echo "Testing QNAS deployment process..."
	mkdir -p /tmp/qnap-test
	cp qnap-build/* /tmp/qnap-test/
	cd /tmp/qnap-test && ls -la
	@echo "✅ QNAS deployment test completed in /tmp/qnap-test"

# Show QNAS build information
qnap-info:
	@echo "QNAS Build Information:"
	@echo "  Target Platform: QNAP NAS (x86_64/ARM64)"
	@echo "  Compatible: QTS 4.x+, QuTS hero"
	@echo "  Build Tools: Docker + Multi-stage builds"
	@echo "  Output: Static binaries (no dependencies)"
	@echo ""
	@echo "Quick Commands:"
	@echo "  make qnap         - Build QNAS version"
	@echo "  make qnap-verbose - Verbose build"
	@echo "  make qnap-test     - Build and test"
	@echo "  make qnap-clean    - Clean artifacts"
	@echo "  make qnap-all      - Build all architectures"

# Help for QNAS targets
help-qnap:
	@echo "QNAS Build Targets:"
	@echo ""
	@echo "Build Commands:"
	@echo "  qnap              - Build QNAS version (AMD64)"
	@echo "  qnap-verbose      - Build with verbose output"
	@echo "  qnap-version      - Build with specific version"
	@echo "  qnap-amd64        - Build AMD64 only"
	@echo "  qnap-arm64        - Build ARM64 binaries"
	@echo "  qnap-compose      - Multi-architecture build"
	@echo "  qnap-all          - Build all architectures"
	@echo ""
	@echo "Testing & Quality:"
	@echo "  qnap-test         - Build and test binaries"
	@echo "  qnap-deploy-test  - Test deployment process"
	@echo ""
	@echo "Package & Deploy:"
	@echo "  qnap-package      - Create deployment package"
	@echo "  qnap-push         - Build and push to registry"
	@echo ""
	@echo "Maintenance:"
	@echo "  qnap-clean        - Clean build artifacts"
	@echo "  qnap-info         - Show QNAS build info"
	@echo "  help-qnap         - Show this help"
	@echo ""
	@echo "Examples:"
	@echo "  make qnap VERSION=1.0.0"
	@echo "  make qnap-push REGISTRY=registry.example.com"
	@echo "  make qnap-all VERSION=latest"
