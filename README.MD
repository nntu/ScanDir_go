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