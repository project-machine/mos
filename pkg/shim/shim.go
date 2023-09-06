package shim

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"unsafe"

	efi "github.com/canonical/go-efilib"
	"github.com/project-machine/mos/pkg/obj"
)

var nativeEndian binary.ByteOrder

// from cert_table at
// https://github.com/rhboot/shim/blob/aedb8470bd673385139ac3189ecd9edf4794af16/shim.c#L49
type shimCertTable struct {
	AuthSize     uint32
	DeAuthSize   uint32
	AuthOffset   uint32
	DeAuthOffset uint32
}

// vendorDBSectionHeader - write a header for the .vendor_cert section
// of a shim executable. The vendor_section header is native endian.
// it represents the 'cert_table' type
//
//	https://github.com/rhboot/shim/blob/aedb8470bd673385139ac3189ecd9edf4794af16/cert.S
func vendorDBSectionHeader(dbSize int, dbxSize int) ([]byte, error) {
	const dbOffset = uint32(16)
	var b bytes.Buffer
	err := binary.Write(&b, nativeEndian,
		shimCertTable{
			AuthSize:     uint32(dbSize),
			DeAuthSize:   uint32(dbxSize),
			AuthOffset:   uint32(dbOffset),
			DeAuthOffset: uint32(dbSize) + uint32(dbOffset),
		})
	return b.Bytes(), err
}

func VendorDBSectionWrite(writer io.Writer, sigdb, sigdbx efi.SignatureDatabase) error {
	dbBuf, err := sigdb.Bytes()
	if err != nil {
		return err
	}

	dbxBuf, err := sigdbx.Bytes()
	if err != nil {
		return err
	}

	header, err := vendorDBSectionHeader(len(dbBuf), len(dbxBuf))
	if err != nil {
		return err
	}

	for _, b := range []*[]byte{&header, &dbBuf, &dbxBuf} {
		if n, err := writer.Write(*b); err != nil {
			return err
		} else if n != len(*b) {
			return fmt.Errorf("Wrote only %d bytes of %d", n, *b)
		}
	}
	return nil
}

// SetVendorDB - set the VendorDB inside existing file "shim"
//
//	with provided db and dbx
func SetVendorDB(shim string, db, dbx efi.SignatureDatabase) error {
	fp, err := ioutil.TempFile("", "setvendordb")
	if err != nil {
		return err
	}

	if err := VendorDBSectionWrite(fp, db, dbx); err != nil {
		return err
	}

	fp.Close()

	sections := []obj.SectionInput{
		{Name: ".vendor_cert", VMA: 0xb4000, Path: fp.Name()}}

	if err := obj.SetSections(shim, sections...); err != nil {
		return err
	}

	return nil
}

func init() {
	buf := [2]byte{}
	*(*uint16)(unsafe.Pointer(&buf[0])) = uint16(0xABCD)

	switch buf {
	case [2]byte{0xCD, 0xAB}:
		nativeEndian = binary.LittleEndian
	case [2]byte{0xAB, 0xCD}:
		nativeEndian = binary.LittleEndian
	default:
		panic("Could not determine native endianness.")
	}
}
