// deleter.go
//go:build deleter

package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/sirupsen/logrus"
)

type deleteFilter struct {
	SizeZero bool
	Exts     []string // normalized, e.g. ".tmp"
}

// configureDB configures database connection settings for optimal performance
func configureDB(db *sql.DB, phase string, workers int) {
	switch phase {
	case "delete":
		// Deletion: Mixed read/write operations
		db.SetMaxOpenConns(2)
		db.SetMaxIdleConns(1)
		db.SetConnMaxLifetime(30 * time.Minute)
	default:
		// Default configuration
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
	}
}

// deleteWithOptimizedQueries performs deletion with optimized database queries
func deleteWithOptimizedQueries(ctx context.Context, db *sql.DB, cleanPath string) (foldersDeleted, filesDeleted int64, err error) {
	// Prepare LIKE pattern for subdirectory matching
	likePath := cleanPath
	if !strings.HasSuffix(likePath, "/") {
		likePath += "/"
	}
	likePath += "%" // e.g., /path/to/folder/%

	// Use transaction for atomic operations
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	// Delete files using optimized query with proper indexes
	fileResult, err := tx.ExecContext(ctx, `
		DELETE FROM fs_files
		WHERE path = ?
		   OR dir_path = ?
		   OR dir_path LIKE ?
		   OR path LIKE ?`,
		cleanPath, cleanPath, likePath, likePath)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to delete from fs_files: %w", err)
	}
	filesDeleted, _ = fileResult.RowsAffected()

	// Delete folders using optimized query
	folderResult, err := tx.ExecContext(ctx, `
		DELETE FROM fs_folders
		WHERE path = ? OR path LIKE ?`,
		cleanPath, likePath)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to delete from fs_folders: %w", err)
	}
	foldersDeleted, _ = folderResult.RowsAffected()

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		return 0, 0, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return foldersDeleted, filesDeleted, nil
}

// validateDeletePath performs safety checks before deletion
func validateDeletePath(path string) error {
	if path == "" {
		return fmt.Errorf("path cannot be empty")
	}

	if path == "/" || path == "\\" {
		return fmt.Errorf("cannot delete root directory")
	}

	// Add more validation as needed
	return nil
}

func normalizeExtList(extsCSV string) []string {
	extsCSV = strings.TrimSpace(extsCSV)
	if extsCSV == "" {
		return nil
	}
	parts := strings.Split(extsCSV, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if !strings.HasPrefix(p, ".") {
			p = "." + p
		}
		p = strings.ToLower(p)
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

func buildInPlaceholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimRight(strings.Repeat("?,", n), ",")
}

func deleteByConditions(ctx context.Context, db *sql.DB, logger *logrus.Logger, basePath string, filter deleteFilter, deleteDisk bool, dryRun bool, limit int) (int64, int64, int64, error) {
	// returns: dbDeleted, diskDeleted, errors
	var dbDeleted int64
	var diskDeleted int64
	var errCount int64

	cleanPath := filepath.ToSlash(basePath)
	likePath := cleanPath
	if !strings.HasSuffix(likePath, "/") {
		likePath += "/"
	}
	likePath += "%"

	clauses := []string{
		`(path = ? OR path LIKE ? OR dir_path = ? OR dir_path LIKE ?)`,
	}
	args := []any{cleanPath, likePath, cleanPath, likePath}

	if filter.SizeZero {
		clauses = append(clauses, `size = 0`)
	}
	if len(filter.Exts) > 0 {
		clauses = append(clauses, fmt.Sprintf(`LOWER(fileExt) IN (%s)`, buildInPlaceholders(len(filter.Exts))))
		for _, e := range filter.Exts {
			args = append(args, e)
		}
	}

	query := fmt.Sprintf(`
		SELECT id, path
		FROM fs_files
		WHERE %s
		ORDER BY id
	`, strings.Join(clauses, " AND "))

	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("query filter delete: %w", err)
	}
	defer rows.Close()

	type idPath struct {
		id   int64
		path string
	}
	const commitBatch = 1000
	batch := make([]idPath, 0, commitBatch)

	flush := func() error {
		if len(batch) == 0 {
			return nil
		}

		if dryRun {
			dbDeleted += int64(len(batch)) // "would delete" in DB
			batch = batch[:0]
			return nil
		}

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()

		delStmt, err := tx.PrepareContext(ctx, `DELETE FROM fs_files WHERE id = ?`)
		if err != nil {
			return err
		}
		defer delStmt.Close()

		for _, it := range batch {
			if deleteDisk {
				// Windows chấp nhận path dạng '/', giữ nguyên; nhưng vẫn clean nhẹ.
				p := filepath.Clean(filepath.FromSlash(it.path))
				if rmErr := os.Remove(p); rmErr != nil {
					// Nếu file không tồn tại, vẫn cho xóa record DB để "dọn" index.
					if !os.IsNotExist(rmErr) {
						errCount++
						logger.WithFields(logrus.Fields{
							"path":  it.path,
							"error": rmErr.Error(),
						}).Warn("Failed to delete file from disk")
						continue
					}
				} else {
					diskDeleted++
				}
			}

			if _, err := delStmt.ExecContext(ctx, it.id); err != nil {
				errCount++
				logger.WithFields(logrus.Fields{
					"id":    it.id,
					"path":  it.path,
					"error": err.Error(),
				}).Warn("Failed to delete row from database")
				continue
			}
			dbDeleted++
		}

		if err := tx.Commit(); err != nil {
			return err
		}
		batch = batch[:0]
		return nil
	}

	for rows.Next() {
		var id int64
		var p string
		if err := rows.Scan(&id, &p); err != nil {
			errCount++
			logger.WithError(err).Warn("Failed to scan fs_files row")
			continue
		}
		batch = append(batch, idPath{id: id, path: p})
		if len(batch) >= commitBatch {
			if err := flush(); err != nil {
				return dbDeleted, diskDeleted, errCount, err
			}
		}
	}
	if err := rows.Err(); err != nil {
		return dbDeleted, diskDeleted, errCount, err
	}
	if err := flush(); err != nil {
		return dbDeleted, diskDeleted, errCount, err
	}

	return dbDeleted, diskDeleted, errCount, nil
}

// ----------------------------
// main (deleter) - Optimized Version
// ----------------------------

func main() {
	// Initialize structured logging
	logger := logrus.New()
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})
	logger.SetLevel(logrus.InfoLevel)

	logger.Info("Go Deleter (Optimized SQLite Edition) starting...")

	// Parse command line arguments
	dbFile := flag.String("dbfile", "", "Path to the scan.db file (e.g., ./output_scans/scan_....db)")
	pathToDelete := flag.String("path", "", "Absolute path of the file/folder scope (required for safety)")
	verbose := flag.Bool("verbose", false, "Enable verbose logging")
	dryRun := flag.Bool("dry-run", false, "Show what would be deleted without actually deleting")
	deleteDisk := flag.Bool("delete-disk", false, "Delete matching files from disk (DANGEROUS). If false: delete from DB only")

	// Filter mode
	filterSizeZero := flag.Bool("size-zero", false, "Filter: only files with size = 0")
	filterExts := flag.String("ext", "", "Filter: file extensions, comma-separated (e.g. .tmp,.log,.bak)")
	limit := flag.Int("limit", 0, "Safety: max number of files to delete (0 = no limit)")
	flag.Parse()

	if *dbFile == "" {
		logger.Fatal("Error: -dbfile flag is required.")
	}
	if *pathToDelete == "" {
		logger.Fatal("Error: -path flag is required (safety).")
	}

	if *verbose {
		logger.SetLevel(logrus.DebugLevel)
	}

	// Validate and normalize path
	cleanPath, err := filepath.Abs(*pathToDelete)
	if err != nil {
		logger.Fatalf("Failed to resolve absolute path: %v", err)
	}
	cleanPath = filepath.ToSlash(cleanPath)

	// Validate path for safety
	if err := validateDeletePath(cleanPath); err != nil {
		logger.Fatalf("Path validation failed: %v", err)
	}

	filter := deleteFilter{
		SizeZero: *filterSizeZero,
		Exts:     normalizeExtList(*filterExts),
	}
	useFilter := filter.SizeZero || len(filter.Exts) > 0

	logger.WithFields(logrus.Fields{
		"dbPath":     *dbFile,
		"scopePath":  cleanPath,
		"dryRun":     *dryRun,
		"deleteDisk": *deleteDisk,
		"filterMode": useFilter,
		"sizeZero":   filter.SizeZero,
		"extFilters": filter.Exts,
		"limit":      *limit,
	}).Info("Starting deletion job")

	// Open database with optimized settings
	db, err := openDBSQLite(*dbFile)
	if err != nil {
		logger.WithError(err).Fatalf("Failed to open database %s", *dbFile)
	}
	defer db.Close()

	// Configure database for optimal deletion performance
	configureDB(db, "delete", 1)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// FILTER MODE: delete by conditions within scopePath
	if useFilter {
		startTime := time.Now()
		dbDeleted, diskDeleted, errCount, err := deleteByConditions(ctx, db, logger, cleanPath, filter, *deleteDisk, *dryRun, *limit)
		if err != nil {
			logger.WithError(err).Fatal("Filter deletion failed")
		}
		duration := time.Since(startTime)
		logger.WithFields(logrus.Fields{
			"dbDeleted":   dbDeleted,
			"diskDeleted": diskDeleted,
			"errors":      errCount,
			"duration_ms": duration.Milliseconds(),
			"itemsPerSec": float64(dbDeleted) / duration.Seconds(),
		}).Info("Filter deletion completed")
		return
	}

	// Check if path exists in database before deletion (for dry run)
	if *dryRun {
		logger.Info("DRY RUN: Checking what would be deleted...")

		// Check folders
		var folderCount int64
		folderQuery := `SELECT COUNT(*) FROM fs_folders WHERE path = ? OR path LIKE ? || '%'`
		db.QueryRowContext(ctx, folderQuery, cleanPath, cleanPath).Scan(&folderCount)

		// Check files
		var fileCount int64
		fileQuery := `SELECT COUNT(*) FROM fs_files WHERE path = ? OR dir_path = ? OR dir_path LIKE ? || '%'`
		db.QueryRowContext(ctx, fileQuery, cleanPath, cleanPath, cleanPath).Scan(&fileCount)

		logger.WithFields(logrus.Fields{
			"foldersFound": folderCount,
			"filesFound":   fileCount,
		}).Info("DRY RUN: Would delete these items")
		return
	}

	// Perform deletion with optimized queries
	startTime := time.Now()
	foldersDeleted, filesDeleted, err := deleteWithOptimizedQueries(ctx, db, cleanPath)
	if err != nil {
		logger.WithError(err).Fatal("Deletion failed")
	}

	duration := time.Since(startTime)

	// Log success with detailed metrics
	logger.WithFields(logrus.Fields{
		"foldersDeleted": foldersDeleted,
		"filesDeleted":   filesDeleted,
		"totalItems":     foldersDeleted + filesDeleted,
		"duration":       duration.Milliseconds(),
		"itemsPerSecond": float64(foldersDeleted+filesDeleted) / duration.Seconds(),
	}).Info("Deletion completed successfully")

	if *deleteDisk {
		logger.Warn("NOTE: -delete-disk is currently supported only in filter mode (e.g. -size-zero / -ext). Without filters, this run deleted from DB only.")
	} else {
		logger.Info("NOTE: This tool deletes from DB. Use -delete-disk with filter flags to delete files from disk as well.")
	}
}

// Legacy main function for backward compatibility
func mainLegacy() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.Printf("Go Deleter (SQLite Edition) starting...")

	dbFile := flag.String("dbfile", "", "Path to the scan.db file (e.g., ./output_scans/scan_....db)")
	pathToDelete := flag.String("path", "", "Absolute path of the file or folder to delete (from DB only)")
	flag.Parse()

	if *dbFile == "" {
		log.Fatal("Error: -dbfile flag is required.")
	}
	if *pathToDelete == "" {
		log.Fatal("Error: -path flag is required.")
	}

	// 1. Chuẩn hóa Path
	cleanPath, _ := filepath.Abs(*pathToDelete)
	cleanPath = filepath.ToSlash(cleanPath)

	log.Printf("Attempting to delete from DB: %s", cleanPath)

	// 2. Mở DB
	db, err := openDBSQLite(*dbFile)
	if err != nil {
		log.Fatalf("Failed to open db %s: %v", *dbFile, err)
	}
	defer db.Close()

	ctx := context.Background()

	// 3. Thực hiện Xóa trong Transaction
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		log.Fatalf("Failed to start transaction: %v", err)
	}
	defer tx.Rollback() // Rollback nếu có lỗi

	// Chuẩn bị path cho LIKE
	likePath := cleanPath
	if !strings.HasSuffix(likePath, "/") {
		likePath += "/"
	}
	likePath += "%" // vd: /path/to/folder/%

	var totalFiles, totalFolders int64

	// Xóa (hard-delete) các file
	resFile, err := tx.ExecContext(ctx, `
		DELETE FROM fs_files
		WHERE path = ? OR dir_path = ? OR dir_path LIKE ?`,
		cleanPath, cleanPath, likePath)
	if err != nil {
		log.Fatalf("Failed to delete from fs_files: %v", err)
	}
	totalFiles, _ = resFile.RowsAffected()

	// Xóa (hard-delete) các thư mục
	resFolder, err := tx.ExecContext(ctx, `
		DELETE FROM fs_folders
		WHERE path = ? OR path LIKE ?`,
		cleanPath, likePath)
	if err != nil {
		log.Fatalf("Failed to delete from fs_folders: %v", err)
	}
	totalFolders, _ = resFolder.RowsAffected()

	// 4. Commit
	if err := tx.Commit(); err != nil {
		log.Fatalf("Failed to commit transaction: %v", err)
	}

	log.Printf("Success! Hard-deleted %d folders and %d files from the database.", totalFolders, totalFiles)
	log.Println("NOTE: This tool does NOT delete files from the actual filesystem.")
}
