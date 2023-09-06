package shim

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestShimHead(t *testing.T) {
	var dbSize, dbxSize uint32 = 925, 0
	headerSize := uint32(16)
	header, err := vendorDBSectionHeader(int(dbSize), int(dbxSize))
	if err != nil {
		t.Errorf("VendorDBSectionHeader failed: %v", err)
	}

	ctable := shimCertTable{}
	if err := binary.Read(bytes.NewReader(header), nativeEndian, &ctable); err != nil {
		t.Errorf("binary.Read into ctable failed: %v", err)
	}

	if ctable.AuthOffset != headerSize {
		t.Errorf("ctable.AuthSize found %d, expected %d", ctable.AuthOffset, headerSize)
	}

	if ctable.DeAuthOffset != (headerSize + dbSize) {
		t.Errorf("ctable.DeAuthOffset found %d, expected %d", ctable.DeAuthOffset, headerSize+dbxSize)
	}

	if ctable.AuthSize != dbSize {
		t.Errorf("ctable.AuthSize found %d, expected %d", ctable.AuthSize, dbSize)
	}

	if ctable.DeAuthSize != dbxSize {
		t.Errorf("ctable.AuthSize found %d, expected %d", ctable.DeAuthSize, dbxSize)
	}
}
