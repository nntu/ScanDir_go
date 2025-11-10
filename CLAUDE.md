# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build Commands

### Build All Binaries with Docker
```bash
make
```
This builds Docker image and extracts three executables: `scanner`, `deleter`, and `reporter`.

### Build Individual Components
```bash
# Build scanner only
go build -tags scanner -trimpath -o scanner .

# Build deleter only
go build -tags deleter -trimpath -o deleter .

# Build reporter only
go build -tags reporter -trimpath -o reporter .
```

### Clean Build Artifacts
```bash
make clean
```

## Development Workflow

### Running the Scanner
```bash
./scanner
```
- Reads configuration from `config.ini`
- Creates timestamped SQLite database in `output_dir` (default: `./output_scans`)
- Runs in two phases: metadata scan → duplicate hashing

### Running the Deleter
```bash
./deleter -dbfile <path_to_scan.db> -path <absolute_path_to_delete>
```
- Removes entries from database only (does NOT delete actual files)
- Use absolute paths for the path parameter

### Running the Reporter
```bash
# Console output (default)
./reporter -dbfile <path_to_scan.db>

# Excel report
./reporter -dbfile <path_to_scan.db> -format excel -output report.xlsx

# HTML report with top 50 files
./reporter -dbfile <path_to_scan.db> -format html -output report.html -topn 50
```

## Architecture

### Multi-Binary Structure
This project uses Go build tags to create three separate executables from shared code:

- **scanner**: Main filesystem scanning tool (`//go:build scanner`)
- **deleter**: Database entry removal tool (`//go:build deleter`)
- **reporter**: Report generation tool (`//go:build reporter`)

### Shared Components
- `common_types.go`: Data structures shared across all tools
- `common_config.go`: Configuration loading from `config.ini`
- `common_db.go`: Database initialization and connection handling
- `stat_*.go`: Platform-specific file stat operations

### Scanner Architecture (Two-Phase)

**Phase 1: Metadata Scan**
- Concurrent directory traversal using worker pool
- Collects file metadata (name, path, size, modification time)
- Batch inserts into SQLite database for performance
- Uses `fs_folders` and `fs_files` tables with foreign key relationships

**Phase 2: Duplicate Hashing**
- Queries database for files with identical sizes (potential duplicates)
- Calculates MD5 hashes using worker pool
- Updates database with hash values for duplicate detection

### Database Schema
- `fs_folders`: Hierarchical folder structure with parent_id foreign key
- `fs_files`: File metadata with folder_id foreign key, size, and hash_value
- Optimized indexes on path, size, and hash_value fields

### Configuration
The `config.ini` file controls:
- `[output]`: Database output directory
- `[scan]`: Batch size, worker count, excluded directories
- `[paths]`: Root paths to scan with tags (format: `/path/to/folder:TagName`)

### Performance Optimizations
- SQLite WAL mode for concurrent access
- Batch database inserts (configurable batch size)
- Worker pool for parallel scanning and hashing
- Platform-specific file stat operations
- Database connection pooling and transaction management