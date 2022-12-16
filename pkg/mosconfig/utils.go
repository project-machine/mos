package mosconfig

import (
	"fmt"
	"io"
	"os"
)

func PathExists(d string) bool {
	_, err := os.Stat(d)
	if err != nil && os.IsNotExist(err) {
		return false
	}
	return true
}

func EnsureDir(dir string) error {
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return fmt.Errorf("Failed creating directory %q: %w", dir, err)
	}
	return nil
}

//  If src is a symlink, copies content, not link.
//  TODO - copy the permissions.  For now it just makes all new files
//  0644 which is what we want anyway.
func CopyFileBits(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dest, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}
	return out.Close()
}
