# Công cụ Duyệt Hệ thống File (Phiên bản SQLite)

Đây là một công cụ Go hiệu suất cao dùng để quét các hệ thống file lớn, tính toán hash (MD5) của từng file, và lưu trữ kết quả vào một file database **SQLite** duy nhất cho mỗi lần chạy.

Mục tiêu chính là tạo ra một cơ sở dữ liệu "snapshot" (ảnh chụp nhanh) của hệ thống file để phục vụ cho việc **phân tích file trùng lặp** và **tạo báo cáo dung lượng**.

Kiến trúc này được tối ưu cho tốc độ quét, loại bỏ hoàn toàn độ trễ mạng bằng cách ghi trực tiếp vào một file .db cục bộ.

## Tính năng

* **Quét song song (Concurrent Scanning)**: Sử dụng worker pool để quét và băm (hash) file trên nhiều luồng.
* **Ghi vào SQLite**: Mọi kết quả được ghi vào một file `.db` duy nhất (ví dụ: `scan_20251024_130000.db`).
* **Tối ưu Hashing**: Tự động tính toán MD5 cho các file có nội dung (size > 0).
* **Tối ưu Ghi**: Sử dụng `WAL mode`, `PRAGMA` tối ưu, và `Batch Inserts` bên trong `Transaction` để đạt tốc độ ghi SQLite nhanh nhất.
* **Đơn giản hóa**: Loại bỏ hoàn toàn logic theo dõi thay đổi (`deleted_at`), chỉ tập trung vào việc tạo snapshot.
* **Kiểm tra trùng lặp có thể chạy lại**: Có tool `checkdup` để rebuild `duplicate_groups` + `is_duplicate` và theo dõi tiến độ trong `duplicate_runs`.
* **Báo cáo nâng cao**: Có `reporter_opt` (build tag `reporter_optimized`) để report nhanh hơn (cache + query tối ưu).

## Nguyên lý hoạt động

Ứng dụng hoạt động qua hai giai đoạn riêng biệt:

1.  **Giai đoạn 1: Quét Metadata:**
    *   Quét đệ quy các đường dẫn gốc được định nghĩa trong `config.ini`.
    *   Thu thập metadata của file (tên, đường dẫn, kích thước, thời gian sửa đổi) cho tất cả các file.
    *   Chèn metadata này vào database SQLite theo lô để đạt hiệu suất cao.

2.  **Giai đoạn 2: Băm (Hashing):**
    *   Truy vấn database để tìm các file có kích thước giống hệt nhau, vì đây là các file có khả năng trùng lặp.
    *   Đối với các file trùng lặp tiềm năng này, nó tính toán hash MD5 của từng file.
    *   Cập nhật database với các hash đã tính toán.

## Cấu hình

Ứng dụng được cấu hình qua file `config.ini`:

*   `[output]`:
    *   `output_dir`: Thư mục mà các file database SQLite kết quả sẽ được lưu.
*   `[scan]`:
    *   `BATCH_SIZE`: Số lượng bản ghi file được nhóm lại để chèn vào database một lần.
    *   `MAX_WORKERS`: Số lượng worker song song để quét thư mục.
    *   `EXCLUDE_DIRS`: Danh sách các tên thư mục bị loại trừ khỏi việc quét, ngăn cách bởi dấu phẩy.
*   `[paths]`:
    *   `root1`, `root2`, v.v.: Các đường dẫn gốc cần quét. Định dạng là `key = /path/to/folder:TagName`.

## Sử dụng

Dự án này cung cấp các chương trình thực thi (Go build tags):

- `scanner` (tag `scanner`): quét metadata + hashing duplicate (Giai đoạn 1/2)
- `checkdup` (tag `checkdup`): rebuild duplicate data (có thể chạy lại, có progress)
- `deleter` (tag `deleter`): xoá record trong DB theo đường dẫn hoặc theo điều kiện (tuỳ chọn xoá file thật)
- `reporter` (tag `reporter`): report cơ bản
- `reporter_opt` (tag `reporter_optimized`): report tối ưu

1.  **Cấu hình:** Chỉnh sửa file `config.ini` để chỉ định các đường dẫn bạn muốn quét.

2.  **Build binaries cục bộ (Makefile):**
    Để build các chương trình thực thi cục bộ, sử dụng:
    ```bash
    make build-local
    ```
    Điều này sẽ tạo ra `scanner`, `checkdup`, `deleter`, `reporter`, `reporter_opt` trong thư mục gốc của dự án (tuỳ thuộc vào HĐH/CGO).

    **Lưu ý Windows + SQLite**: dự án dùng `github.com/mattn/go-sqlite3` nên cần **CGO**. Nếu bạn build mà bị lỗi kiểu `CGO_ENABLED=0 ... sqlite3 requires cgo`, hãy build bằng Docker (phần dưới) hoặc cài GCC (MSYS2/mingw) và build với `CGO_ENABLED=1`.

3.  **Chạy Scanner:**
    Thực thi chương trình `scanner` từ dòng lệnh. Điều này sẽ quét các đường dẫn đã cấu hình và tạo một database SQLite.
    ```bash
    ./scanner
    ```

4.  **Chạy Deleter:**
    Sử dụng `deleter` để xoá dữ liệu.

    - **Chế độ Path (mặc định)**: xoá record trong DB theo phạm vi thư mục/file.
    ```bash
    ./deleter -dbfile <path_to_scan.db> -path <absolute_path_to_delete>
    ```
    Ví dụ:
    ```bash
    ./deleter -dbfile ./output_scans/scan_20251024_130000.db -path /path/to/folder/or/file
    ```

    - **Chế độ Filter**: xoá theo điều kiện trong phạm vi `-path`:
        - `-size-zero`: chỉ file `size=0`
        - `-ext ".tmp,.bak"`: lọc theo `fileExt`
        - `-limit N`: giới hạn số lượng để an toàn
        - `-delete-disk`: **xoá file thật trên ổ đĩa** (NGUY HIỂM) + xoá record DB tương ứng

    - **Xuất danh sách xoá (CSV/TSV)**:
        - `-list-out <file>`: xuất danh sách `type,id,path`
        - `-list-format csv|tsv` (mặc định `csv`)

    Ví dụ (dry-run + xuất CSV):
    ```bash
    ./deleter -dbfile ./output_scans/scan_20251024_130000.db -path /path/to/scope -ext ".tmp" -dry-run -list-out delete_list.csv
    ```

5.  **Chạy Reporter:**
    Tạo báo cáo (các file lớn nhất, file trùng lặp) ở nhiều định dạng khác nhau.
    ```bash
    ./reporter -dbfile <path_to_scan.db> -format <excel|html|console> [-output <output_file>] [-topn <number>]
    ```
    Ví dụ:
    *   Tạo báo cáo console (mặc định):
        ```bash
        ./reporter -dbfile ./output_scans/scan_20251024_130000.db
        ```
    *   Tạo báo cáo Excel:
        ```bash
        ./reporter -dbfile ./output_scans/scan_20251024_130000.db -format excel -output my_report.xlsx
        ```
    *   Tạo báo cáo HTML với 50 file lớn nhất:
        ```bash
        ./reporter -dbfile ./output_scans/scan_20251024_130000.db -format html -output my_report.html -topn 50
        ```

    *   Reporter tối ưu:
        ```bash
        ./reporter_opt -dbfile ./output_scans/scan_20251024_130000.db -format json -output report.json
        ```

6. **Chạy CheckDup (chạy lại phát hiện trùng lặp):**

```bash
./checkdup -dbfile ./output_scans/scan_20251024_130000.db -reset=true
```

Tool sẽ rebuild `duplicate_groups` + cập nhật `is_duplicate`. Tiến độ được ghi vào bảng `duplicate_runs` trong DB.

7.  **Phân tích:** Khi việc quét hoàn tất, một file database SQLite mới sẽ được tạo trong `output_dir`. Bạn có thể sử dụng bất kỳ client SQLite nào (như DBeaver, DB Browser for SQLite) để mở file và phân tích dữ liệu.

## Mẹo phát triển: chạy đúng với Go build tags

Nếu bạn dùng `go run`, hãy chạy trên **package** và chỉ định tag, ví dụ:

```bash
go run -tags reporter . -dbfile ./output_scans/scan_20251024_130000.db -format console
```

## Cross-compiling cho Linux (với Docker)

Nếu bạn cần chạy `scanner`, `deleter`, và `reporter` trên hệ thống Linux, bạn có thể sử dụng Docker để cross-compile ứng dụng. Điều này đảm bảo rằng các binary được xây dựng trong một môi trường nhất quán.

1.  **Đảm bảo `Dockerfile` có mặt:**
    Đảm bảo rằng `Dockerfile` trong thư mục gốc của dự án của bạn có nội dung sau:

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

2.  **Build và Extract Binaries (sử dụng Makefile):**
    Sử dụng `Makefile` được cung cấp để build Docker image và extract binaries:
    ```bash
    make
    ```
    Lệnh này sẽ xử lý việc build Docker image và sao chép các chương trình thực thi ra khỏi container.

Sau các bước này, bạn sẽ tìm thấy các chương trình thực thi trong thư mục dự án của mình, sẵn sàng để chuyển và chạy trên hệ thống Linux mục tiêu.

## Build cho QNAP (Dockerfile.qnap)

Repo có `Dockerfile.qnap` để build ra:
- `./qnap-build/bin/{scanner,deleter,reporter,reporter_opt,checkdup}`
- `./qnap-build/qnap-scandir-<VERSION>-<arch>.tar.gz`

Lưu ý: `.dockerignore` đã exclude `output_dir/` để tránh đưa DB lớn vào Docker build context.