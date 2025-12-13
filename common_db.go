// common_db.go
//go:build scanner || deleter || reporter || reporter_optimized || checkdup

package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	_ "github.com/mattn/go-sqlite3" // Import driver SQLite
)

// ensureSchemaUpgrades: apply non-destructive schema upgrades for older DB files.
// Safe to call multiple times.
func ensureSchemaUpgrades(db *sql.DB) error {
	// If fs_folders doesn't exist, nothing to do.
	var dummy int
	err := db.QueryRow(`SELECT 1 FROM sqlite_master WHERE type='table' AND name='fs_folders' LIMIT 1;`).Scan(&dummy)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return fmt.Errorf("check sqlite_master(fs_folders): %w", err)
	}

	cols := map[string]bool{}
	rows, err := db.Query(`PRAGMA table_info(fs_folders);`)
	if err != nil {
		return fmt.Errorf("PRAGMA table_info(fs_folders): %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return fmt.Errorf("scan PRAGMA table_info(fs_folders): %w", err)
		}
		cols[name] = true
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate PRAGMA table_info(fs_folders): %w", err)
	}

	// Add folder stats columns if missing (non-destructive).
	if !cols["size"] {
		if _, err := db.Exec(`ALTER TABLE fs_folders ADD COLUMN size BIGINT NOT NULL DEFAULT 0;`); err != nil {
			return fmt.Errorf("ALTER TABLE fs_folders ADD COLUMN size: %w", err)
		}
	}
	if !cols["number_files"] {
		if _, err := db.Exec(`ALTER TABLE fs_folders ADD COLUMN number_files INTEGER NOT NULL DEFAULT 0;`); err != nil {
			return fmt.Errorf("ALTER TABLE fs_folders ADD COLUMN number_files: %w", err)
		}
	}
	if !cols["subtree_size"] {
		if _, err := db.Exec(`ALTER TABLE fs_folders ADD COLUMN subtree_size BIGINT NOT NULL DEFAULT 0;`); err != nil {
			return fmt.Errorf("ALTER TABLE fs_folders ADD COLUMN subtree_size: %w", err)
		}
	}
	if !cols["subtree_files"] {
		if _, err := db.Exec(`ALTER TABLE fs_folders ADD COLUMN subtree_files INTEGER NOT NULL DEFAULT 0;`); err != nil {
			return fmt.Errorf("ALTER TABLE fs_folders ADD COLUMN subtree_files: %w", err)
		}
	}

	// Helpful indexes (no-op if already exists).
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_folder_size ON fs_folders (size DESC);`); err != nil {
		return fmt.Errorf("CREATE INDEX idx_folder_size: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_folder_number_files ON fs_folders (number_files DESC);`); err != nil {
		return fmt.Errorf("CREATE INDEX idx_folder_number_files: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_folder_subtree_size ON fs_folders (subtree_size DESC);`); err != nil {
		return fmt.Errorf("CREATE INDEX idx_folder_subtree_size: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_folder_subtree_files ON fs_folders (subtree_files DESC);`); err != nil {
		return fmt.Errorf("CREATE INDEX idx_folder_subtree_files: %w", err)
	}

	return nil
}

// makeDBSQLite (dùng cho scanner)
func makeDBSQLite(dbPath string) (*sql.DB, error) {
	_ = os.Remove(dbPath) // Xóa file cũ nếu tồn tại

	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_synchronous=NORMAL&_cache_size=1000000", dbPath)

	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec("PRAGMA busy_timeout = 5000;"); err != nil {
		return nil, err
	}
	if err := ensureSchemaUpgrades(db); err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // Ghi đơn luồng trong Phase 1
	return db, nil
}

// openDBSQLite (dùng cho deleter)
func openDBSQLite(dbPath string) (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_synchronous=NORMAL", dbPath)

	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec("PRAGMA busy_timeout = 5000;"); err != nil {
		return nil, err
	}
	if err := ensureSchemaUpgrades(db); err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(10)
	return db, nil
}

// initDDL (dùng cho scanner - Optimized Version)
func initDDL(ctx context.Context, db *sql.DB) error {
	stmts := []string{
		// Bảng Thư mục
		`CREATE TABLE fs_folders (
		  id INTEGER PRIMARY KEY AUTOINCREMENT,
		  parent_id INTEGER,
		  path TEXT NOT NULL,
		  name TEXT NOT NULL,
		  st_mtime DATETIME NOT NULL,
		  loaithumuc TEXT,
		  size BIGINT NOT NULL DEFAULT 0,
		  number_files INTEGER NOT NULL DEFAULT 0,
		  subtree_size BIGINT NOT NULL DEFAULT 0,
		  subtree_files INTEGER NOT NULL DEFAULT 0,

		  FOREIGN KEY (parent_id) REFERENCES fs_folders (id)
		)`,
		`CREATE UNIQUE INDEX idx_folder_path ON fs_folders (path);`,
		`CREATE INDEX idx_folder_parent_id ON fs_folders (parent_id);`,
		`CREATE INDEX idx_folder_mtime ON fs_folders (st_mtime DESC);`,
		`CREATE INDEX idx_folder_loaithumuc ON fs_folders (loaithumuc);`,
		`CREATE INDEX idx_folder_size ON fs_folders (size DESC);`,
		`CREATE INDEX idx_folder_number_files ON fs_folders (number_files DESC);`,
		`CREATE INDEX idx_folder_subtree_size ON fs_folders (subtree_size DESC);`,
		`CREATE INDEX idx_folder_subtree_files ON fs_folders (subtree_files DESC);`,

		// Bảng Files
		`CREATE TABLE fs_files (
		  id INTEGER PRIMARY KEY AUTOINCREMENT,
		  folder_id INTEGER NOT NULL,
		  path TEXT NOT NULL,
		  dir_path TEXT NOT NULL,
		  filename TEXT NOT NULL,
		  fileExt TEXT,
		  size BIGINT NOT NULL,
		  st_mtime DATETIME NOT NULL,
		  hash_value TEXT NULL, -- Sẽ được tool 'hasher' cập nhật
		  is_duplicate BOOLEAN DEFAULT 0, -- Đánh dấu file là duplicate
		  loaithumuc TEXT,
		  thumuc TEXT,

		  FOREIGN KEY (folder_id) REFERENCES fs_folders (id)
		)`,
		// Basic indexes
		`CREATE UNIQUE INDEX idx_file_path ON fs_files (path);`,
		`CREATE INDEX idx_file_size ON fs_files (size) WHERE size > 0;`,
		`CREATE INDEX idx_file_hash ON fs_files (hash_value) WHERE hash_value IS NOT NULL;`,
		`CREATE INDEX idx_file_is_duplicate ON fs_files (is_duplicate) WHERE is_duplicate = 1;`,

		// Optimized performance indexes
		`CREATE INDEX idx_file_size_hash_null ON fs_files (size) WHERE hash_value IS NULL;`,
		`CREATE INDEX idx_file_folder_id_size ON fs_files (folder_id, size DESC);`,
		`CREATE INDEX idx_file_mtime ON fs_files (st_mtime DESC);`,
		`CREATE INDEX idx_file_hash_size ON fs_files (hash_value, size) WHERE hash_value IS NOT NULL;`,
		`CREATE INDEX idx_file_dir_path ON fs_files (dir_path);`,
		`CREATE INDEX idx_file_extension ON fs_files (fileExt) WHERE fileExt IS NOT NULL;`,
		`CREATE INDEX idx_file_loaithumuc ON fs_files (loaithumuc);`,

		// Composite indexes for common query patterns
		`CREATE INDEX idx_file_folder_loaithumuc_size ON fs_files (folder_id, loaithumuc, size DESC);`,
		`CREATE INDEX idx_file_size_mtime ON fs_files (size DESC, st_mtime DESC);`,
		`CREATE INDEX idx_file_hash_null_size ON fs_files (hash_value IS NULL, size DESC) WHERE hash_value IS NULL;`,
		`CREATE INDEX idx_file_hash_duplicate ON fs_files (hash_value, is_duplicate) WHERE hash_value IS NOT NULL AND is_duplicate = 1;`,

		// Bảng Duplicate Groups (để query nhanh hơn)
		`CREATE TABLE duplicate_groups (
		  hash_value TEXT PRIMARY KEY,
		  file_count INTEGER NOT NULL,
		  total_size BIGINT NOT NULL,
		  first_seen DATETIME NOT NULL,
		  last_updated DATETIME NOT NULL
		)`,
		`CREATE INDEX idx_duplicate_groups_size ON duplicate_groups (total_size DESC);`,
		`CREATE INDEX idx_duplicate_groups_count ON duplicate_groups (file_count DESC);`,

		// Bảng theo dõi tiến độ chạy check-duplicate (để rerun/monitor)
		`CREATE TABLE duplicate_runs (
		  id INTEGER PRIMARY KEY AUTOINCREMENT,
		  started_at DATETIME NOT NULL,
		  finished_at DATETIME NULL,
		  status TEXT NOT NULL, -- running|done|failed
		  total_groups INTEGER DEFAULT 0,
		  processed_groups INTEGER DEFAULT 0,
		  processed_files INTEGER DEFAULT 0,
		  processed_size BIGINT DEFAULT 0,
		  last_hash_value TEXT NULL,
		  note TEXT NULL
		)`,
		`CREATE INDEX idx_duplicate_runs_status ON duplicate_runs (status);`,
		`CREATE INDEX idx_duplicate_runs_started_at ON duplicate_runs (started_at DESC);`,
	}

	for i, s := range stmts {
		if _, err := db.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("failed to execute statement %d (%s): %w", i, s, err)
		}
	}
	return nil
}
