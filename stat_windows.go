//go:build windows && (scanner || deleter)

package main

import (
	"os"
	"os/user"
)

// Windows-specific: best-effort atime/ctime via fi.ModTime();
func statInfo(fi os.FileInfo) StatInfo {
	mtime := fi.ModTime()
	atime := mtime
	ctime := mtime

	username := "0"
	// Attempt to get username, but it might not be directly comparable to Unix UIDs.
	if u, err := user.Current(); err == nil {
		username = u.Username
	}

	return StatInfo{
		Size: fi.Size(), Atime: atime, Mtime: mtime, Ctime: ctime, Username: username,
	}
}
