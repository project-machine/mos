package trust

import (
	"bytes"
	"crypto"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/canonical/go-efilib"
	"github.com/canonical/tcglog-parser"
	"github.com/project-machine/mos/pkg/utils"
)

const ShimLockGUID = "605dab50-e046-4300-abb6-3dd810dd8b23"
const ShimVendordbGUID = "00000000-0000-0000-0000-000000000000"
const SBAT = "sbat,1,2021030218\012"

// Using DBX data from current ovmf_vars.fd in bootkit.
// Revisit if ovmf or dbx changes. We need to eventually manage dbx.
const DBX = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
const DBXGuid = "a3a8baa01d04a848bc87c36d121b5e3d"

type efiVarInfo struct {
	varguid efi.GUID
	hashed  []byte
}

func getCert(certfile string) ([]byte, error) {
	// read cert data from certfile and put in DER
	certData, err := os.ReadFile(certfile)
	if err != nil {
		return nil, fmt.Errorf("Failed to read %q: (%w)", certfile, err)
	}

	block, _ := pem.Decode(certData)
	if block == nil {
		return nil, errors.New("failed to pem.Decode cert")
	}
	return block.Bytes, nil
}

func getCertGUID(guidfile string) (efi.GUID, error) {
	// Read and decode the guid for the cert
	cGuid, err := os.ReadFile(guidfile)
	if err != nil {
		return efi.GUID{}, fmt.Errorf("Failed to read %q: (%w)", guidfile, err)
	}
	certGuid, err := efi.DecodeGUIDString(string(cGuid))
	if err != nil {
		return efi.GUID{}, fmt.Errorf("Failed to decode the guid in %q: (%w)", guidfile, err)
	}

	return certGuid, nil
}

func createSigData(unicodeName string, varGUID efi.GUID, certfile, guidfile string) ([]byte, error) {
	// Read cert.pem and guid
	certBytes, err := getCert(certfile)
	if err != nil {
		return nil, err
	}
	certGuid, err := getCertGUID(guidfile)
	if err != nil {
		return nil, err
	}

	// Create SignatureData
	sd := efi.SignatureData{Owner: certGuid, Data: certBytes}
	var b bytes.Buffer
	sd.Write(&b)
	return tcglog.ComputeEFIVariableDataDigest(crypto.SHA256, unicodeName, varGUID, b.Bytes()), nil
}

func createESLfromCert(unicodeName string, varGUID efi.GUID, certfile, guidfile string) ([]byte, error) {
	// Read cert.pem and guid
	certBytes, err := getCert(certfile)
	if err != nil {
		return nil, err
	}
	certGuid, err := getCertGUID(guidfile)
	if err != nil {
		return nil, err
	}

	// create a SignatureList
	sl := efi.SignatureList{Type: efi.CertX509Guid, Header: []byte{}}
	sl.Signatures = append(sl.Signatures, &efi.SignatureData{Owner: certGuid, Data: certBytes})

	var b bytes.Buffer
	sl.Write(&b)
	return tcglog.ComputeEFIVariableDataDigest(crypto.SHA256, unicodeName, varGUID, b.Bytes()), nil
}

func createESLfromHash(unicodeName string, varGUID efi.GUID, hdata []byte, hguid efi.GUID) ([]byte, error) {
	sl := efi.SignatureList{Type: efi.CertSHA256Guid, Header: []byte{}}
	sl.Signatures = append(sl.Signatures, &efi.SignatureData{Owner: hguid, Data: hdata})
	var b bytes.Buffer
	sl.Write(&b)
	return tcglog.ComputeEFIVariableDataDigest(crypto.SHA256, unicodeName, varGUID, b.Bytes()), nil
}

func getHash(unicodeName string, varGuid efi.GUID, keysetPath string) ([]byte, error) {

	switch unicodeName {
	case "SecureBoot", "MokListTrusted":
		efiData := []byte{1}
		return tcglog.ComputeEFIVariableDataDigest(crypto.SHA256, unicodeName, varGuid, efiData), nil

	case "SbatLevel":
		return tcglog.ComputeEFIVariableDataDigest(crypto.SHA256, unicodeName, varGuid, []byte(SBAT)), nil

	case "PK", "KEK", "db":
		dir := strings.ToLower(unicodeName)
		certfile := filepath.Join(keysetPath, "uefi-"+dir+"/cert.pem")
		guidfile := filepath.Join(keysetPath, "uefi-"+dir+"/guid")

		hashed, err := createESLfromCert(unicodeName, varGuid, certfile, guidfile)
		if err != nil {
			return nil, err
		}
		return hashed, nil

	case "dbx":
		hdata, err := hex.DecodeString(DBX)
		if err != nil {
			return nil, err
		}

		guiddata, err := hex.DecodeString(DBXGuid)
		if err != nil {
			return nil, fmt.Errorf("Failed to decode the dbx guid: (%w)", err)
		}

		r := bytes.NewReader(guiddata)
		hguid, err := efi.ReadGUID(r)
		if err != nil {
			return nil, err
		}

		hashed, err := createESLfromHash(unicodeName, varGuid, hdata, hguid)
		if err != nil {
			return nil, err
		}
		return hashed, nil

	case "separator":
		return tcglog.ComputeSeparatorEventDigest(crypto.SHA256, tcglog.SeparatorEventNormalValue), nil

	case "shim-cert":
		certfile := filepath.Join(keysetPath, "uefi-db/cert.pem")
		guidfile := filepath.Join(keysetPath, "uefi-db/guid")
		hashed, err := createSigData("db", varGuid, certfile, guidfile)
		if err != nil {
			return nil, err
		}
		return hashed, nil

	case "production", "tpm", "limited":
		certfile := filepath.Join(keysetPath, "uki-"+unicodeName, "cert.pem")
		certBytes, err := getCert(certfile)
		if err != nil {
			return nil, err
		}

		certGuid, err := efi.DecodeGUIDString(ShimVendordbGUID)
		if err != nil {
			return nil, err
		}

		// Create SignatureData
		sd := efi.SignatureData{Owner: certGuid, Data: certBytes}
		var b bytes.Buffer
		sd.Write(&b)
		return tcglog.ComputeEFIVariableDataDigest(crypto.SHA256, "vendor_db", varGuid, b.Bytes()), nil

	default:
		return nil, nil
	}
}

func ComputePCR7(keysetName string) ([]byte, []byte, []byte, error) {
	var pcr7Val = []byte{00, 00, 00, 00, 00, 00, 00, 00, 00, 00, 00,
		00, 00, 00, 00, 00, 00, 00, 00, 00, 00, 00,
		00, 00, 00, 00, 00, 00, 00, 00, 00, 00}

	// List of uefi Secure boot vars that get measured.
	// It includes certs that get measured at boot. UKI certs are measured
	// separately since we have 3 possible. Also measured in this order.
	var uefiMeasured = []string{"SecureBoot", "PK", "KEK", "db", "dbx", "separator",
		"shim-cert", "SbatLevel", "MokListTrusted"}

	shimLockGuid, err := efi.DecodeGUIDString(ShimLockGUID)
	if err != nil {
		return nil, nil, nil, err
	}
	var efiSBInfo = map[string]*efiVarInfo{
		"SecureBoot":     {efi.GlobalVariable, nil},
		"PK":             {efi.GlobalVariable, nil},
		"KEK":            {efi.GlobalVariable, nil},
		"db":             {efi.ImageSecurityDatabaseGuid, nil},
		"dbx":            {efi.ImageSecurityDatabaseGuid, nil},
		"separator":      {hashed: nil},
		"shim-cert":      {efi.ImageSecurityDatabaseGuid, nil},
		"SbatLevel":      {shimLockGuid, nil},
		"MokListTrusted": {shimLockGuid, nil},
	}

	// List of UKI certs that can get measured.
	// The value will be the resulting pcr7 value when the cert is extended.
	var ukiKeys = map[string][]byte{"production": nil, "limited": nil, "tpm": nil}

	moskeysetPath, err := utils.GetMosKeyPath()
	if err != nil {
		return nil, nil, nil, err
	}

	keysetPath := filepath.Join(moskeysetPath, keysetName)
	if !utils.PathExists(keysetPath) {
		return nil, nil, nil, fmt.Errorf("Unknown keyset '%s', cannot find keyset at path: %q", keysetName, keysetPath)
	}

	// First calculate the uefi vars and certs in order they are extended into pcr7.
	for _, k := range uefiMeasured {
		efiSBInfo[k].hashed, err = getHash(k, efiSBInfo[k].varguid, keysetPath)
		if err != nil {
			return nil, nil, nil, err
		}
		h := crypto.SHA256.New()
		h.Write(pcr7Val)
		h.Write(efiSBInfo[k].hashed)
		pcr7Val = h.Sum(nil)
	}

	// Now extend in the 3 different possible uki signing keys.
	// This will result in 3 different pcr7 values, one for each possible uki key.
	for uki, _ := range ukiKeys {
		ukiHash, err := getHash(uki, efi.ImageSecurityDatabaseGuid, keysetPath)
		if err != nil {
			return nil, nil, nil, err
		}
		h := crypto.SHA256.New()
		h.Write(pcr7Val)
		h.Write(ukiHash)
		ukiKeys[uki] = h.Sum(nil)
	}
	return ukiKeys["production"], ukiKeys["limited"], ukiKeys["tpm"], nil
}
