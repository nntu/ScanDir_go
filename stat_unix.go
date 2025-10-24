//go:build !windows && (scanner || deleter)

package main

import (
	"os"
	"os/user"
	"strconv"
	"syscall"
	"time"
)

// Linux-only: best-effort atime/ctime via unix.Stat_t; fallback to mtime if fields missing.
// Improved: obtain real UID from Stat_t and lookup username; fallback to numeric uid string.
func statInfo(fi os.FileInfo) StatInfo {
	mtime := fi.ModTime()
	atime := mtime
	ctime := mtime

	var uid uint32 = 0
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		if st.Atim.Sec != 0 {
			atime = time.Unix(int64(st.Atim.Sec), int64(st.Atim.Nsec))
		}
		if st.Ctim.Sec != 0 {
			ctime = time.Unix(int64(st.Ctim.Sec), int64(st.Ctim.Nsec))
		}
		uid = st.Uid
	}

	// Lookup username by UID. If lookup fails (e.g., no /etc/passwd inside container),
	// return numeric uid as string instead of a fixed "0".
	username := strconv.FormatUint(uint64(uid), 10)
	if u, err := user.LookupId(strconv.FormatUint(uint64(uid), 10)); err == nil {
		if u.Username != "" {
			username = u.Username
		}
	}

	return StatInfo{
		Size: fi.Size(), Atime: atime, Mtime: mtime, Ctime: ctime, Username: username,
	}
}
