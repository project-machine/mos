package main

import (
	"fmt"
	"io"
	"os"

	"compress/gzip"

	"github.com/project-machine/mos/pkg/initrd"
	cli "github.com/urfave/cli/v2"
)

var _ = initrd.Identity

var initrdCmd = cli.Command{
	Name: "initrd",
	Subcommands: []*cli.Command{
		&cli.Command{
			Name:      "join",
			ArgsUsage: "output-initrd cpio-ish [cpio-ish ...]",
			Action:    doInitrdJoin,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:      "microcode",
					Aliases:   []string{"m"},
					Usage:     "Put file in as microcode",
					TakesFile: true,
					Value:     "",
				},
				&cli.BoolFlag{
					Name:    "gzip",
					Aliases: []string{"z"},
					Usage:   "Compress with gzip",
				},
			},
		},
	},
}

func isGzipped(buf []byte) bool {
	if len(buf) < 2 {
		return false
	}
	return buf[0] == 0x1f && buf[1] == 0x8b
}

/*
// i started working with this, trying to think about how to support
// zstd and gzip decompression.
// the thought to use a multiReader is possibly useful to "replay" the buff.
// also thought to try a type assertion to a seeker and seek backwards the
// amount that was read if it can.
//
// last, https://github.com/golang/go/issues/51092
func uncompressedReader(r io.Reader) (io.Reader, error) {
	var bbuf bytes.Buffer
	// read into a buf from r. buf is smallest size to be
	// able to determine if it is compressed.
	buf := make([]byte, 2, 2)
	n, err := r.Read(buf)
	if err != nil && err != io.EOF {
		return nil, err
	} else if n < len(buf) {
		bbuf.Write(buf)
		return &bbuf, nil
	}

	// if we have a seeker, we can avoid the multireader
	rSeeker, ok := r.(io.ReadSeeker)
	if ok {
		if _, err := rSeeker.Seek((int64)(-n), io.SeekCurrent); err != nil {
			return nil, err
		}
	} else {
		var bbuf bytes.Buffer
		bbuf.Write(buf)
		r = io.MultiReader(&bbuf, r)
	}

	type nr func(io.Reader) (*gzip.Reader, error)
	f := []nr{gzip.NewReader}
	for _, f := range []nr{gzip.NewReader} {
	}
	gzip.NewReader
	// if you can gzip.NewReader(bytes.Buffer(buf))
	return nil, nil
}
*/

func uncompressedPathReader(path string) (io.ReadCloser, error) {
	r, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	buf := make([]byte, 2, 2)
	n, err := r.Read(buf)
	if err != nil {
		r.Close()
		return nil, err
	}

	if _, err := r.Seek(0, io.SeekStart); err != nil {
		r.Close()
		return nil, err
	}

	if n < 2 || !isGzipped(buf) {
		return r, nil
	}

	zRead, err := gzip.NewReader(r)
	if err != nil {
		r.Close()
		return nil, err
	}
	return zRead, nil
}

func doInitrdJoin(ctx *cli.Context) error {
	args := ctx.Args().Slice()

	if len(args) < 2 {
		return fmt.Errorf("Got %d args, expect 2 or more", len(args))
	}
	output := args[0]

	var outWriter io.Writer = os.Stdout
	if output != "-" {
		w, err := os.Create(output)
		if err != nil {
			return err
		}
		defer w.Close()
		outWriter = w
	}

	if mcpath := ctx.String("microcode"); mcpath != "" {
		mcReader, err := uncompressedPathReader(ctx.String("microcode"))
		if err != nil {
			return err
		}
		_, err = io.Copy(outWriter, mcReader)
		if err != nil {
			return err
		}

		mcReader.Close()
	}

	fmt.Fprintf(os.Stderr, "Wrote to %s\n", output)

	return nil
}
