// deleter.go
//go:build deleter

package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/sirupsen/logrus"
)

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
	pathToDelete := flag.String("path", "", "Absolute path of the file or folder to delete (from DB only)")
	verbose := flag.Bool("verbose", false, "Enable verbose logging")
	dryRun := flag.Bool("dry-run", false, "Show what would be deleted without actually deleting")
	flag.Parse()

	if *dbFile == "" {
		logger.Fatal("Error: -dbfile flag is required.")
	}
	if *pathToDelete == "" {
		logger.Fatal("Error: -path flag is required.")
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

	logger.WithFields(logrus.Fields{
		"dbPath":   *dbFile,
		"filePath": cleanPath,
		"dryRun":   *dryRun,
	}).Info("Attempting to delete from database")

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

	logger.Info("NOTE: This tool does NOT delete files from the actual filesystem, only from the database.")
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
