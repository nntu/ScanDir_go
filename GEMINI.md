# Filesystem Indexer (SQLite Edition)

## Project Overview
This project is a high-performance Go-based filesystem indexer designed to scan large directory structures, capture file metadata, and calculate MD5 hashes for duplicate detection. All data is stored in a local, timestamped SQLite database, optimized for write performance.

**Key Capabilities:**
*   **Concurrent Scanning:** Utilizes worker pools for parallel directory traversal and file hashing.
*   **SQLite Storage:** Stores scan snapshots in portable `.db` files, enabling offline analysis without network latency.
*   **Two-Phase Operation:**
    1.  **Metadata Scan:** Rapidly indexes file paths, sizes, and timestamps.
    2.  **Duplicate Hashing:** Identifies files with identical sizes and computes hashes only for potential duplicates.
*   **Reporting:** Includes tools to generate duplicate reports and storage analytics in Console, Excel, or HTML formats.

## Architecture
The project is structured as a mono-repo producing three distinct executables using Go build tags:

*   **`scanner`**: The core indexing engine.
*   **`deleter`**: A utility to remove entries from the database (does *not* delete files from disk).
*   **`reporter`**: A tool to query the database and generate user-friendly reports.

**Shared Components:**
*   `common_types.go`, `common_config.go`, `common_db.go`: shared logic for data structures, config parsing, and DB interactions.
*   `stat_*.go`: Platform-specific optimizations for file system operations.

## Build & Run

### Prerequisites
*   Go 1.24+
*   Docker (optional, for cross-compilation)
*   Make

### Building
The project uses a `Makefile` to manage builds.

**Local Build (Host OS):**
```bash
make build-local
```
This creates `scanner`, `deleter`, and `reporter` binaries in the root directory.

**Cross-Compile for Linux (via Docker):**
```bash
make
```
This builds a Docker image and extracts the binaries, ensuring compatibility with Linux environments (e.g., QNAP NAS).

**Build for QNAP NAS:**
```bash
make qnap
```

### Configuration
Create or modify `config.ini` in the working directory:

```ini
[output]
output_dir = ./output_scans

[scan]
BATCH_SIZE = 5000
MAX_WORKERS = 4
EXCLUDE_DIRS = .git,node_modules

[paths]
; Format: unique_key = /path/to/scan:Tag
root1 = D:\Projects:MyProjects
```

### Usage

**1. Run Scanner:**
```bash
./scanner
```
Generates a database file in `output_dir` (e.g., `scan_20251024_120000.db`).

**2. Generate Report:**
```bash
# Console summary
./reporter -dbfile ./output_scans/scan_2025...db

# Excel Export
./reporter -dbfile ./output_scans/scan_2025...db -format excel -output report.xlsx
```

**3. Database Cleanup (Optional):**
```bash
./deleter -dbfile ./output_scans/scan_2025...db -path "/path/to/remove/from/index"
```

## Development Conventions

*   **Build Tags:** Code specific to a tool is isolated using `//go:build <tag>`. e.g., `scanner.go` starts with `//go:build scanner`.
*   **Database:** Uses `mattn/go-sqlite3`. Interactions are optimized with WAL mode and batch transactions.
*   **Cross-Platform:** `stat_windows.go` and `stat_unix.go` handle OS-specific file attribute retrieval.
