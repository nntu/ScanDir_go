# Tên của Docker image và container
IMAGE_NAME := go-scan-app
CONTAINER_NAME := go-scan-app-temp

# Tên của các binary sẽ được trích xuất
SCANNER_BIN := scanner
DELETER_BIN := deleter
REPORTER_BIN := reporter

# Các target mặc định và giả (phony targets)
.PHONY: all build-image create-container copy-scanner copy-deleter copy-reporter remove-container extract-binaries clean

# Target mặc định: build image và trích xuất binary
all: build-image extract-binaries

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
	-rm -f $(SCANNER_BIN) $(DELETER_BIN) $(REPORTER_BIN)
	@echo "Cleanup complete."
