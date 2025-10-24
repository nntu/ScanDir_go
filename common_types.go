// common_types.go
//go:build scanner || deleter

package main

import (
	"database/sql"
	"time"
)

// Config (dùng chung)
type Config struct {
	OutputDir  string
	BatchSize  int
	MaxWorkers int
	Exclude    map[string]struct{}
	Paths      [][2]string // (root_path, loaithumuc)
}

// StatInfo (dùng chung)
type StatInfo struct {
	Size     int64
	Atime    time.Time
	Mtime    time.Time
	Ctime    time.Time
	Username string
}

// --- Structs cho Scanner (Phase 1) ---

// DirInsertReq (dùng cho scanner)
type DirInsertReq struct {
	ParentID   int64 // 0 nếu là root
	EntryPath  string
	EntryName  string
	Info       StatInfo
	LoaiThuMuc string
	Resp       chan int64 // Channel để nhận về ID của thư mục vừa insert
}

// FileRow (dùng cho scanner)
type FileRow struct {
	FolderID   int64
	Path       string
	DirPath    string
	Filename   string
	FileExt    string
	Size       int64
	Mtime      time.Time
	LoaiThuMuc string
	ThuMuc     string
}

// DbMsg (dùng cho scanner)
type DbMsg struct {
	InsertDir   *DirInsertReq
	InsertFiles []FileRow
	Shutdown    bool
}

// --- Structs cho Hashing (Phase 2) ---

// FileToHash (struct cho worker)
type FileToHash struct {
	ID   int64
	Path string
}

// HashResult (struct cho worker)
type HashResult struct {
	ID   int64
	Hash sql.NullString
	Err  error
}
