package trust

import "fmt"

type EAPolicyVersion int

func (v EAPolicyVersion) String() string {
	return fmt.Sprintf("%04d", v)
}

type NVIndex int

func (i NVIndex) String() string {
	return fmt.Sprintf("0x%08x", int(i))
}

const PolicyVersion EAPolicyVersion = 1
const TpmLayoutVersion int = 3
const (
	// This is the password for TPM administration.
	TPM2IndexPassword NVIndex = 0x1500001
	// Version of 'TPM layout'.  Any time a nvindex is added,
	// removed, or changed, bump this version.
	TPM2IndexTPMVersion NVIndex = 0x1500002
	// This is the EA policy version.  Policies to read
	// LUKS nvindex are depending on the version.
	TPM2IndexEAVersion NVIndex = 0x1500020
	// These are the provisioned certificate and key.
	TPM2IndexCert NVIndex = 0x1500021
	TPM2IndexKey  NVIndex = 0x1500022
	// The LUKS password for the sbs
	TPM2IndexSBSKey NVIndex = 0x1500030
	// The LUKS password for OS filesystems
	TPM2IndexOSKey NVIndex = 0x1500040
)

const TPM_PCRS_DEF = "sha256:7"

// SBF - Secure Block Factory
const SBFPartitionName = "sbf"
const SBFMapperName = "secureBootFlash"

// PBF - Plaintext Block Factory
const PBFPartitionName = "pbf"
const PBFMountpoint = "/factory/pbf"

const SignDataDir = "/pcr7data"

// PBFPartitionTypeID - 01A3E19F-9FEA-ED47-92C2-E75639FF5601
var PBFPartitionTypeID = [16]byte{
	0x9f, 0xe1, 0xa3, 0x01, 0xea, 0x9f, 0x47, 0xed, 0x92, 0xc2, 0xe7, 0x56, 0x39, 0xff, 0x56, 0x01}

// SBFPartitionTypeID is 01A3E19F-9FEA-ED47-92C2-E75639FF5602
var SBFPartitionTypeID = [16]byte{
	0x9f, 0xe1, 0xa3, 0x01, 0xea, 0x9f, 0x47, 0xed, 0x92, 0xc2, 0xe7, 0x56, 0x39, 0xff, 0x56, 0x02}

const MiB, GiB = uint64(1024 * 1024), uint64(1024 * 1024 * 1024)

var Version string
var BootkitVersion string
