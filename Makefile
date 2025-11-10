# Tên của Docker image và container
IMAGE_NAME := go-scan-app
CONTAINER_NAME := go-scan-app-temp

# Tên của các binary sẽ được trích xuất
SCANNER_BIN := scanner
DELETER_BIN := deleter
REPORTER_BIN := reporter
REPORTER_OPT_BIN := reporter_opt

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
