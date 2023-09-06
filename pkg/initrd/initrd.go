package initrd

import (
	"bytes"
	"fmt"
	"io"
	"os"
)

// - just join existing cpios (compressed or not)
// - always compress
// - support creation of cpio from a dir
// - support firmware - must be first and uncompressed
// - filter out duped filenames - requires uncompressing and recompressing

type Compression int

const (
	Undetermined Compression = iota
	Identity
	Gzip
)

type CpioReader interface {
	Read(p []byte) (n int, err error)
	SetCompression(Compression) error
	Compression() Compression
}

type DedupedReader struct {
	readers []CpioReader
	seen    map[string]bool
}

func (d *DedupedReader) Read(p []byte) (n int, err error) {
	return 0, nil
}

func (d *DedupedReader) SetCompression(comp Compression) error {
	for _, r := range d.readers {
		if err := r.SetCompression(comp); err != nil {
			return err
		}
	}
	return nil
}

// Compression - return the compression for this reader
//
//	if all of the readers are the same, it can return that compression
//	otherwise it has to return identity.
func (d *DedupedReader) Compression() Compression {
	var cur, last Compression
	for i, r := range d.readers {
		cur = r.Compression()
		if i == 0 {
			last = cur
		}
		if cur != last {
			return Identity
		}
		last = cur
	}
	return last
}

func NewDedupedReader(readers []CpioReader) (*DedupedReader, error) {
	r := &DedupedReader{readers: readers}
	return r, nil
}

type CpioFileReader struct {
	path   string
	reader io.Reader
	comp   Compression
	unused bytes.Buffer
}

func (r *CpioFileReader) Read(p []byte) (int, error) {
	return 0, fmt.Errorf("implement me please")
}

func (r *CpioFileReader) SetCompression(comp Compression) error {
	r.comp = comp
	return fmt.Errorf("Implement me please")
}

func (r *CpioFileReader) Compression() Compression {
	return r.comp
}

func NewCpioFileReader(path string) (*CpioFileReader, error) {
	r := &CpioFileReader{path: path}
	fp, err := os.Open(path)
	if err != nil {
		return r, nil
	}
	r.reader = fp
	return r, nil
}
