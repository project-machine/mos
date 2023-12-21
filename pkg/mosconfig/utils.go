package mosconfig

import (
	"fmt"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/apex/log"
	"github.com/pkg/errors"
)

// Taken from stacker's squashfs package
// Takes /proc/self/uid_map contents as one string
// Returns true if this is a uidmap representing the whole host
// uid range.
func uidmapIsHost(oneline string) bool {
	oneline = strings.TrimSuffix(oneline, "\n")
	if len(oneline) == 0 {
		return false
	}
	lines := strings.Split(oneline, "\n")
	if len(lines) != 1 {
		return false
	}
	words := strings.Fields(lines[0])
	if len(words) != 3 || words[0] != "0" || words[1] != "0" || words[2] != "4294967295" {
		return false
	}

	return true
}

// chown of symlinks in overlay on top of squashfuse fails.  So, if we are using
// squashfuse, then find all symlinks, delete and re-reate them.
func fixupSymlinks(dir string) error {
	err := filepath.Walk(dir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			log.Warnf("fixupSymlinks: failed accessing %q: %v (continuing)", path, err)
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			dest, err := os.Readlink(path)
			if err != nil {
				log.Warnf("fixupSymlinks: readlink failed on %q: %w (continuing)", path, err)
				return nil
			}
			err = os.Remove(path)
			if err != nil {
				return err
			}
			// func Symlink(oldname, newname string) error
			err = os.Symlink(dest, path)
			if err != nil {
				return err
			}
			stat := info.Sys().(*syscall.Stat_t)
			return os.Lchown(path, int(stat.Uid), int(stat.Gid))
		}
		return nil
	})

	return err
}

func dropHashAlg(d string) string {
	s := strings.SplitN(d, ":", 2)
	if len(s) == 2 {
		return s[1]
	}
	return d
}

// pick first unused port >= min
func unusedPort(min int) int {
	port := min
	for {
		s, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err == nil {
			_ = s.Close()
			return port
		}
		port++
	}
}

const zotconf = `
{
  "distSpecVersion": "1.1.0-dev",
  "storage": {
    "rootDirectory": "%s",
    "gc": false
  },
  "http": {
    "address": "127.0.0.1",
    "port": "%d"
  },
  "log": {
    "level": "debug"
  }
}
`

// StartZot starts a new zot server on an unused port.  It returns a cleanup
// function to terminate the zot.
func StartZot(tmpd, cachedir string) (int, func(), error) {
	cleanup := func() {}
	confile := filepath.Join(tmpd, "zot-config.json")
	zotport := unusedPort(20000)

	log.Infof("Starting zot on port %d", zotport)
	conf := fmt.Sprintf(zotconf, cachedir, zotport)
	log.Infof("zot config: %s", conf)
	if err := os.WriteFile(confile, []byte(conf), 0644); err != nil {
		return -1, cleanup, err
	}

	cmd := exec.Command("zot", "serve", confile)
	if err := cmd.Start(); err != nil {
		return -1, cleanup, errors.Wrapf(err, "Failed starting zot")
	}
	cleanup = func() {
		cmd.Process.Kill()
	}

	if err := waitOnZot(zotport); err != nil {
		return -1, cleanup, errors.Wrapf(err, "Zot did not properly start")
	}

	return zotport, cleanup, nil
}

func waitOnZot(port int) error {
	const maxTries = 5
	count := 0

	log.Debugf("Attempting to connect to repo on %d: ", port)
	for {
		err := pingRepo(port)
		if err == nil {
			log.Debugf("... Connected")
			return nil
		}
		if count > maxTries {
			return errors.Wrapf(err, "Failed connecting to our local distribution repo at port %d", port)
		}
		log.Debugf(".")
		time.Sleep(1 * time.Second)
		count += 1
	}
}

func pingRepo(port int) error {
	url := fmt.Sprintf("127.0.0.1:%d", port)
	return PingRepo(url)
}

func pathForPartition(diskPath string, ptnum uint) string {
	base := filepath.Base(diskPath)
	sep := ""
	for _, pre := range []string{"loop", "nvme", "nbd"} {
		if strings.HasPrefix(base, pre) {
			sep = "p"
			break
		}
	}

	return diskPath + sep + fmt.Sprintf("%d", ptnum)
}
