package mosconfig

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func findLock(st *syscall.Stat_t) error {
	content, err := os.ReadFile("/proc/locks")
	if err != nil {
		return fmt.Errorf("failed to read locks file: %w", err)
	}

	for _, line := range strings.Split(string(content), "\n") {
		if len(line) == 0 {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 8 {
			return fmt.Errorf("invalid lock file entry %s", line)
		}

		entries := strings.Split(fields[5], ":")
		if len(entries) != 3 {
			return fmt.Errorf("invalid lock file field %s", fields[5])
		}

		/*
		 * XXX: the kernel prints "fd:01:$ino" for some (all?) locks,
		 * even though the man page we should be able to use fields 0
		 * and 1 as major and minor device types. Let's just ignore
		 * these.
		 */

		ino, err := strconv.ParseUint(entries[2], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid ino %s: %w", entries[2], err)
		}

		if st.Ino != ino {
			continue
		}

		pid := fields[4]
		content, err := os.ReadFile(filepath.Join("/proc", pid, "cmdline"))
		if err != nil {
			return fmt.Errorf("lock owned by pid %s", pid)
		}

		content = bytes.Replace(content, []byte{0}, []byte{' '}, -1)
		return fmt.Errorf("lock owned by pid %s (%s)", pid, string(content))
	}

	return fmt.Errorf("couldn't find who owns the lock")
}

func (mos *Mos) acquireLock() error {
	var err error

	p := filepath.Join(mos.opts.ConfigDir, "manifest.lock")

	mos.lockfile, err = os.Create(p)
	if err != nil {
		return fmt.Errorf("couldn't create lockfile %s: %w", p, err)
	}

	lockMode := syscall.LOCK_EX
	if mos.opts.LayersReadOnly {
		lockMode = syscall.LOCK_SH
	}

	lockErr := syscall.Flock(int(mos.lockfile.Fd()), lockMode|syscall.LOCK_NB)
	if lockErr == nil {
		return nil
	}

	fi, err := mos.lockfile.Stat()
	mos.lockfile.Close()
	mos.lockfile = nil
	if err != nil {
		return fmt.Errorf("couldn't lock or stat lockfile %s: %w", p, err)
	}

	owner := findLock(fi.Sys().(*syscall.Stat_t))
	return fmt.Errorf("couldn't acquire lock on %s: %v: %w", p, owner, lockErr)
}

