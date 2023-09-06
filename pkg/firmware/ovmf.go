package firmware

import (
	"crypto/x509"
	"fmt"
	"os"

	"github.com/project-machine/mos/pkg/cert"
	"github.com/project-machine/mos/pkg/run"

	efi "github.com/canonical/go-efilib"
)

// OVMFPopulateSecureBoot - populate signature data in ovmf-vars file.
// Using the ovmf-vars file in ovmfVarsIn, add the provided platform key
// and the provided kek, db, mok certificates and write the result to
// ovmfVarsOut.
//
// This function requires access to 'virt-fw-vars' provided by the
// virt-firmware python package at https://pypi.org/project/virt-firmware/
func OVMFPopulateSecureBoot(ovmfVarsIn string, ovmfVarsOut string,
	platformKey *efi.SignatureData, kekData, dbData, mokData []*efi.SignatureData) error {

	tmpd, err := os.MkdirTemp("", "OVMFPopulate")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpd)

	sigDataArgs := func(argName string, sigdatum []*efi.SignatureData) ([]string, error) {
		// write each SignatureData to its own file and add argument pointing to it.
		// result is something like []string{"--add-kek", "<guid>", "<tmpfile>"}
		// tmpfiles go in tmpd which is cleaned up above.
		args := []string{}
		for _, sigdata := range sigdatum {
			dcert, err := x509.ParseCertificate(sigdata.Data)
			if err != nil {
				return args, err
			}
			buf, err := cert.PemFromCert(dcert)
			if err != nil {
				return args, err
			}
			tmpf, err := os.CreateTemp(tmpd, "")
			if err != nil {
				return args, err
			}
			if _, err := tmpf.Write(buf); err != nil {
				return args, err
			}
			tmpf.Close()

			args = append(args, argName, sigdata.Owner.String(), tmpf.Name())
		}

		return args, nil
	}

	// custbk virt-fw-vars
	//   "--input=$bkdir/ovmf/ovmf-vars.fd"
	//   "--output=$outdir/ovmf-vars.fd"
	//   "--secure-boot"
	//   "--no-microsoft"
	//   "--set-pk=keydir:$keydir/uefi-pk/"
	//   "--add-kek=keydir:$keydir/uefi-kek/"
	//   "--add-db=keydir:$keydir/uefi-db/"
	args := []string{"virt-fw-vars", "--input=" + ovmfVarsIn, "--output=" + ovmfVarsOut,
		"--secure-boot", "--no-microsoft"}

	for _, c := range [](struct {
		arg string
		sdl []*efi.SignatureData
	}){
		{"--set-pk", []*efi.SignatureData{platformKey}},
		{"--add-kek", kekData},
		{"--add-db", dbData},
		{"--add-mok", mokData},
	} {
		nArgs, err := sigDataArgs(c.arg, c.sdl)
		if err != nil {
			return fmt.Errorf("Failed to get sigData for %s: %w", c.arg, err)
		}
		args = append(args, nArgs...)
	}

	return run.Capture(args...).Error()
}
