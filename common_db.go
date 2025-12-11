// common_db.go
//go:build scanner || deleter || reporter

package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	_ "github.com/mattn/go-sqlite3" // Import driver SQLite
)

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

		  FOREIGN KEY (parent_id) REFERENCES fs_folders (id)
		)`,
		`CREATE UNIQUE INDEX idx_folder_path ON fs_folders (path);`,
		`CREATE INDEX idx_folder_parent_id ON fs_folders (parent_id);`,
		`CREATE INDEX idx_folder_mtime ON fs_folders (st_mtime DESC);`,
		`CREATE INDEX idx_folder_loaithumuc ON fs_folders (loaithumuc);`,

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
	}

	for i, s := range stmts {
		if _, err := db.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("failed to execute statement %d (%s): %w", i, s, err)
		}
	}
	return nil
}
