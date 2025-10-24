// deleter.go
//go:build deleter

package main

import (
	"context"
	"flag"
	"log"
	"path/filepath"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

// ----------------------------
// main (deleter)
// ----------------------------

func main() {
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
