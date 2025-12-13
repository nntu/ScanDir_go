# Filesystem Indexer (SQLite Edition)

This is a high-performance Go tool for scanning large filesystems, calculating hash (MD5) for each file, and storing the results in a single **SQLite** database file per run.

The main goal is to create a database "snapshot" of the filesystem for **duplicate file analysis** and **capacity reporting**.

This architecture is optimized for scanning speed, completely eliminating network latency by writing directly to a local .db file.

## Features

* **Concurrent Scanning**: Uses worker pool to scan and hash files on multiple threads.
* **SQLite Storage**: All results are written to a single `.db` file (e.g., `scan_20251024_130000.db`).
* **Optimized Hashing**: Automatically calculates MD5 for files with content (size > 0).
* **Optimized Writing**: Uses `WAL mode`, optimized `PRAGMA`, and `Batch Inserts` within `Transaction` to achieve the fastest SQLite write speed.
* **Simplified**: Completely removes change tracking logic (`deleted_at`), focusing only on snapshot creation.
* **Rerunnable Duplicate Check**: Includes `checkdup` tool to rebuild `duplicate_groups` + `is_duplicate` and track progress in `duplicate_runs`.
* **Enhanced Reporting**: Includes `reporter_opt` (build tag `reporter_optimized`) for faster reporting (cache + optimized queries).

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

This project provides these executables (Go build tags):

- `scanner` (tag `scanner`): scans metadata + hashes duplicates (Phase 1/2)
- `checkdup` (tag `checkdup`): rebuilds duplicate data (rerunnable, with progress)
- `deleter` (tag `deleter`): deletes records in DB by path or by condition (optionally deletes actual files)
- `reporter` (tag `reporter`): basic reporter
- `reporter_opt` (tag `reporter_optimized`): optimized reporter

1.  **Configure:** Edit the `config.ini` file to specify the paths you want to scan.

2.  **Build local binaries (Makefile):**
    To build local executables, use:
    ```bash
    make build-local
    ```
    This will create `scanner`, `checkdup`, `deleter`, `reporter`, `reporter_opt` in your project root (depending on OS/CGO).

    **Note for Windows + SQLite**: This project uses `github.com/mattn/go-sqlite3` which requires **CGO**. If you encounter build errors like `CGO_ENABLED=0 ... sqlite3 requires cgo`, please build using Docker (see below) or install GCC (MSYS2/mingw) and build with `CGO_ENABLED=1`.

3.  **Run Scanner:**
    Execute the `scanner` application from the command line. This will scan the configured paths and create a SQLite database.
    ```bash
    ./scanner
    ```

4.  **Run Deleter:**
    Use `deleter` to delete data.

    - **Path mode (default)**: deletes records in DB by folder/file scope.
    ```bash
    ./deleter -dbfile <path_to_scan.db> -path <absolute_path_to_delete>
    ```
    Example:
    ```bash
    ./deleter -dbfile ./output_scans/scan_20251024_130000.db -path /path/to/folder/or/file
    ```

    - **Filter mode**: delete by condition within `-path` scope:
        - `-size-zero`: only files with `size=0`
        - `-ext ".tmp,.bak"`: filter by `fileExt`
        - `-limit N`: limit number for safety
        - `-delete-disk`: **actually delete files on disk** (DANGEROUS) + delete corresponding DB records

    - **Export deletion list (CSV/TSV)**:
        - `-list-out <file>`: export list with `type,id,path`
        - `-list-format csv|tsv` (default `csv`)

    Example (dry-run + export CSV):
    ```bash
    ./deleter -dbfile ./output_scans/scan_20251024_130000.db -path /path/to/scope -ext ".tmp" -dry-run -list-out delete_list.csv
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

    *   Optimized reporter:
        ```bash
        ./reporter_opt -dbfile ./output_scans/scan_20251024_130000.db -format json -output report.json
        ```

6. **Run CheckDup (rerun duplicate detection):**

```bash
./checkdup -dbfile ./output_scans/scan_20251024_130000.db -reset=true
```

The tool will rebuild `duplicate_groups` + update `is_duplicate`. Progress is recorded in the `duplicate_runs` table in the DB.

7.  **Analyze:** Once the scan is complete, a new SQLite database file will be created in the `output_dir`. You can use any SQLite client (like DBeaver, DB Browser for SQLite) to open the file and analyze the data.

## Dev tip: Running correctly with Go build tags

If you use `go run`, run it on the **package** and specify the tag, for example:

```bash
go run -tags reporter . -dbfile ./output_scans/scan_20251024_130000.db -format console
```

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

    # Install toolchain (GCC available in manylinux2014), download Go 1.24.0
    RUN curl -fsSL https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz -o /tmp/go.tgz \
     && tar -C /usr/local -xzf /tmp/go.tgz \
     && rm -f /tmp/go.tgz

    WORKDIR /src
    # copy module files first to cache deps (depending on your repo)
    COPY go.mod go.sum ./
    RUN go env -w GOMODCACHE=/go/pkg/mod && go mod download

    # copy code
    COPY . .

    # Build CGO (dynamically linked to glibc baseline 2.17)
    # add -ldflags "-s -w" to reduce size
    RUN go build -tags scanner -trimpath -ldflags="-s -w" -o /out/scanner .
    RUN go build -tags deleter -trimpath -ldflags="-s -w" -o /out/deleter .
    RUN go build -tags reporter -trimpath -ldflags="-s -w" -o /out/reporter .

    # Check required GLIBC symbols (optional)
    RUN ldd /out/scanner && ldd /out/deleter && ldd /out/reporter && (strings -a /out/scanner /out/deleter /out/reporter | grep -o 'GLIBC_[0-9.]*' | sort -u || true)

    # ===== Stage 2: Artifact (extract binary)
    FROM scratch AS artifact
    COPY --from=builder /out/scanner /scanner
    COPY --from=builder /out/deleter /deleter
    COPY --from=builder /out/reporter /reporter
    ```

2.  **Build and Extract Binaries (using Makefile):**
    Use the provided `Makefile` to build the Docker image and extract binaries:
    ```bash
    make
    ```
    This command will handle building the Docker image and copying the executables out of the container.

After these steps, you will find the executables in your project directory, ready to be transferred and run on your target Linux system.

## QNAP build (Dockerfile.qnap)

The repo includes `Dockerfile.qnap` to build:
- `./qnap-build/bin/{scanner,deleter,reporter,reporter_opt,checkdup}`
- `./qnap-build/qnap-scandir-<VERSION>-<arch>.tar.gz`

Note: `.dockerignore` has excluded `output_dir/` to avoid including large databases in the Docker build context.