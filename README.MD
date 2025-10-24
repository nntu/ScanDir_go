# Filesystem Indexer (SQLite Edition)

Dự án này là một tool Go hiệu suất cao dùng để quét (scan) các hệ thống file lớn, tính toán hash (MD5) của từng file, và lưu trữ kết quả vào một file database **SQLite** duy nhất cho mỗi lần chạy.

Mục tiêu chính là tạo ra một cơ sở dữ liệu "snapshot" (ảnh chụp nhanh) của hệ thống file để phục vụ cho việc **phân tích file trùng lặp** và **tạo báo cáo dung lượng**.

Kiến trúc này được tối ưu cho tốc độ quét, loại bỏ hoàn toàn độ trễ mạng bằng cách ghi trực tiếp vào một file .db cục bộ.

## Tính năng

* **Quét song song (Concurrent Scanning)**: Sử dụng worker pool để quét và băm (hash) file trên nhiều luồng.
* **Ghi vào SQLite**: Mọi kết quả được ghi vào một file `.db` duy nhất (ví dụ: `scan_20251024_130000.db`).
* **Tối ưu Hashing**: Tự động tính toán MD5 cho các file có nội dung (size > 0).
* **Tối ưu Ghi**: Sử dụng `WAL mode`, `PRAGMA` tối ưu, và `Batch Inserts` bên trong `Transaction` để đạt tốc độ ghi SQLite nhanh nhất.
* **Đơn giản hóa**: Loại bỏ hoàn toàn logic theo dõi thay đổi (`deleted_at`), chỉ tập trung vào việc tạo snapshot.

## How it Works

The application operates in two distinct phases:

1.  **Phase 1: Metadata Scan:**
    *   Recursively scans the root paths defined in `config.ini`.
    *   Collects file metadata (name, path, size, modification time) for all files.
    *   Inserts this metadata into the SQLite database in batches for high performance.

2.  **Phase 2: Hashing:**
    *   Queries the database to find files with identical sizes, as these are potential duplicates.
    *   For these potential duplicates, it calculates the MD5 hash of each file.
    *   Updates the database with the calculated hashes.

## Configuration

The application is configured via the `config.ini` file:

*   `[output]`:
    *   `output_dir`: The directory where the resulting SQLite database files will be saved.
*   `[scan]`:
    *   `BATCH_SIZE`: The number of file records to batch together for a single database insert.
    *   `MAX_WORKERS`: The number of concurrent workers for scanning directories.
    *   `EXCLUDE_DIRS`: A comma-separated list of directory names to exclude from the scan.
*   `[paths]`:
    *   `root1`, `root2`, etc.: The root paths to be scanned. The format is `key = /path/to/folder:TagName`.

## Usage

1.  **Configure:** Edit the `config.ini` file to specify the paths you want to scan.
2.  **Run:** Execute the application from the command line:
    ```bash
    go run .
    ```
3.  **Analyze:** Once the scan is complete, a new SQLite database file will be created in the `output_dir`. You can use any SQLite client (like DBeaver, DB Browser for SQLite) to open the file and analyze the data.

## Cross-compiling for Linux (with Docker)

If you need to run the `scanner` on a Linux system with an older GLIBC version (e.g., GLIBC 2.21), you can use Docker to cross-compile the application. This ensures that the binary is built against a compatible `libc`.

1.  **Create a `Dockerfile`:**
    Save the following content as `Dockerfile` in the root of your project directory:

    ```dockerfile
    # Use an older Debian distribution for an older GLIBC
    FROM debian:stretch

    # Install necessary packages: git (if needed for go modules), build-essential (for CGO)
    RUN apt-get update && apt-get install -y \
        git \
        build-essential \
        ca-certificates \
        wget \
        && rm -rf /var/lib/apt/lists/*

    # Install Go 1.24.0
    ENV GOLANG_VERSION 1.24.0
    RUN set -eux; \
        ARCH=; \
        case "$(dpkg --print-architecture)" in \
            amd64) ARCH='amd64';; \
            arm64) ARCH='arm64';; \
            *) echo "unsupported architecture"; exit 1 ;; \
        esac; \
        \
        wget -O go.tgz "https://golang.org/dl/go${GOLANG_VERSION}.linux-${ARCH}.tar.gz"; \
        tar -C /usr/local -xzf go.tgz; \
        rm go.tgz; \
        \
        export PATH="/usr/local/go/bin:$PATH"; \
        go version

    ENV PATH="/usr/local/go/bin:$PATH"

    # Set the working directory inside the container
    WORKDIR /app

    # Copy the Go project files into the container
    COPY . .

    # Build the application
    # CGO_ENABLED=1 is crucial here for go-sqlite3
    # GOOS=linux is implicit since we are in a Linux container
    # GOARCH will be the container's architecture
    CMD ["go", "build", "-o", "scanner", "scanner.go"]
    ```

2.  **Build the Docker image:**
    Open your terminal in the project root and run:
    ```bash
    docker build -t scandir-builder .
    ```

3.  **Run the Docker container to compile:**
    This command will compile the `scanner` executable and place it in your project directory on the host machine.
    ```bash
    docker run --rm -v "${PWD}:/app" scandir-builder
    ```

After these steps, you will find the `scanner` executable in your project directory, ready to be transferred and run on your target Linux system.