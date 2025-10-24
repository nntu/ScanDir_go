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

This project provides three main executables: `scanner`, `deleter`, and `reporter`.

1.  **Configure:** Edit the `config.ini` file to specify the paths you want to scan.

2.  **Build and Extract Binaries (using Makefile):**
    To build all executables (`scanner`, `deleter`, `reporter`) and extract them to your current directory, use the provided `Makefile`:
    ```bash
    make
    ```
    This will create `scanner`, `deleter`, and `reporter` binaries in your project root.

3.  **Run Scanner:**
    Execute the `scanner` application from the command line. This will scan the configured paths and create a SQLite database.
    ```bash
    ./scanner
    ```

4.  **Run Deleter:**
    Use the `deleter` to remove entries from the database. **Note: This tool does NOT delete files from the actual filesystem.**
    ```bash
    ./deleter -dbfile <path_to_scan.db> -path <absolute_path_to_delete>
    ```
    Example:
    ```bash
    ./deleter -dbfile ./output_scans/scan_20251024_130000.db -path /path/to/folder/or/file
    ```

5.  **Run Reporter:**
    Generate reports (top largest files, duplicate files) in various formats.
    ```bash
    ./reporter -dbfile <path_to_scan.db> -format <excel|html|console> [-output <output_file>] [-topn <number>]
    ```
    Examples:
    *   Generate console report (default):
        ```bash
        ./reporter -dbfile ./output_scans/scan_20251024_130000.db
        ```
    *   Generate Excel report:
        ```bash
        ./reporter -dbfile ./output_scans/scan_20251024_130000.db -format excel -output my_report.xlsx
        ```
    *   Generate HTML report with top 50 files:
        ```bash
        ./reporter -dbfile ./output_scans/scan_20251024_130000.db -format html -output my_report.html -topn 50
        ```

6.  **Analyze:** Once the scan is complete, a new SQLite database file will be created in the `output_dir`. You can use any SQLite client (like DBeaver, DB Browser for SQLite) to open the file and analyze the data.

## Cross-compiling for Linux (with Docker)

If you need to run the `scanner`, `deleter`, and `reporter` on a Linux system, you can use Docker to cross-compile the application. This ensures that the binaries are built in a consistent environment.

1.  **Ensure `Dockerfile` is present:**
    Make sure the `Dockerfile` in the root of your project directory has the following content:

    ```dockerfile
    # ===== Stage 1: Builder (glibc 2.17 baseline)
    FROM quay.io/pypa/manylinux2014_x86_64 AS builder

    ARG GO_VERSION=1.24.0
    ENV GOROOT=/usr/local/go \
        GOPATH=/go \
        PATH=/usr/local/go/bin:/go/bin:$PATH \
        CGO_ENABLED=1

    # Cài toolchain (GCC có sẵn trong manylinux2014), tải Go 1.24.0
    RUN curl -fsSL https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz -o /tmp/go.tgz \
     && tar -C /usr/local -xzf /tmp/go.tgz \
     && rm -f /tmp/go.tgz

    WORKDIR /src
    # copy module files trước để cache deps (tùy repo của bạn)
    COPY go.mod go.sum ./
    RUN go env -w GOMODCACHE=/go/pkg/mod && go mod download

    # copy code
    COPY . .

    # Build CGO (liên kết động tới glibc baseline 2.17)
    # thêm -ldflags "-s -w" để giảm kích thước
    RUN go build -tags scanner -trimpath -ldflags="-s -w" -o /out/scanner .
    RUN go build -tags deleter -trimpath -ldflags="-s -w" -o /out/deleter .
    RUN go build -tags reporter -trimpath -ldflags="-s -w" -o /out/reporter .

    # Kiểm tra các symbol GLIBC yêu cầu (tuỳ chọn)
    RUN ldd /out/scanner && ldd /out/deleter && ldd /out/reporter && (strings -a /out/scanner /out/deleter /out/reporter | grep -o 'GLIBC_[0-9.]*' | sort -u || true)

    # ===== Stage 2: Artifact (xuất binary)
    FROM scratch AS artifact
    COPY --from=builder /out/scanner /scanner
    COPY --from=builder /out/deleter /deleter
    COPY --from=builder /out/reporter /reporter
    ```

2.  **Build and Extract Binaries (using Makefile):**
    Use the provided `Makefile` to build the Docker image and extract all binaries (`scanner`, `deleter`, `reporter`) to your current directory:
    ```bash
    make
    ```
    This command will handle building the Docker image and copying the executables out of the container.

After these steps, you will find the `scanner`, `deleter`, and `reporter` executables in your project directory, ready to be transferred and run on your target Linux system.