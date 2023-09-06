package cert_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	efi "github.com/canonical/go-efilib"
	. "github.com/project-machine/mos/pkg/cert"
)

var uefiDBPEM = []byte(`-----BEGIN CERTIFICATE-----
MIIDbTCCAlWgAwIBAgIRAJKSmG1+wTSQRQtppwRDJQUwDQYJKoZIhvcNAQELBQAw
TjEOMAwGA1UEChMFQ2lzY28xKjAoBgNVBAsTIVB1enpsZU9TIE1hY2hpbmUgUHJv
amVjdCBzbmFrZW9pbDEQMA4GA1UEAxMHVUVGSSBEQjAeFw0yMzAxMTAxNjM4NTha
Fw00ODAxMTAxNjM4NThaME4xDjAMBgNVBAoTBUNpc2NvMSowKAYDVQQLEyFQdXp6
bGVPUyBNYWNoaW5lIFByb2plY3Qgc25ha2VvaWwxEDAOBgNVBAMTB1VFRkkgREIw
ggEiMA0GCSqGSIb3DQEBAQUAA4IBDwAwggEKAoIBAQCbuCi96/CJ3mfLmLCVLpmo
Ft24qirUJ4FcV8f3Kml6Og68PEfrJsrEwpP79PXMz+edcR7Iwm4Pk5jmRH7DeNAX
802VB2PWC5c/7JiLBn0gE3J3092FwMcAnyMMGAkLIp25tJq5xzJaBpby4+tbZxWt
+Ri2AHYbSS6uQeRtblDA+FL2gzNn2/Rqbx5IM8HqRoldp5ayRoWIYFhBCQOVAva0
ZS4LKmWA2BXKr/fmb4MHV2TO03xy95uA+bxMLKqmnOb1xlqRFhiotKG0ik2Hj88g
aPqz+R7VEf7KAFrMEC4RfTfvkIiGUWnkksbtCIW856Q+hvbTT/IbrZNIdNjuOCBT
AgMBAAGjRjBEMA4GA1UdDwEB/wQEAwIHgDATBgNVHSUEDDAKBggrBgEFBQcDAzAd
BgNVHQ4EFgQU5UFHSGDuAm/ogSz1PzjkJvf3XEcwDQYJKoZIhvcNAQELBQADggEB
AFrVT1POyiXbwirOTagkOgBrjDf8cB1uSiuJi+1yUBJq/aisiQDFy5efVuO9n1az
RgI5fxxx3MB9OZKJvvlk0mi2bwYX0oB+8SE0J9Stb6klIF2T8tgeGhDmRvp20Ch6
yv7/3YnsOGeaxgubaYNue0C7IMQLsl2i+jClbLgnpcMRT2osMJDXY7rbwOEQwYyi
kjn4URm7Wb9dDMeHInIWC3ZmZWXdBeLCqr9vDktWIlQk0iFfN2ezVqUNEbdGijKn
bvlmGsZgFa491cCFtg/6SilS/3LrY6y8vhdcya7ZNra5nYlM1Dc7CG/1qD8ZU2A3
7nd6W+bAEh4sUMjD3PtXuD8=
-----END CERTIFICATE-----
`)

func guidFromString(guidStr string) efi.GUID {
	asGuid, err := efi.DecodeGUIDString(guidStr)
	if err != nil {
		panic(fmt.Sprintf("Failed convert to guid '%s': %v", err, guidStr))
	}
	return asGuid
}

var puzzleDbGuid = guidFromString("326aa6de-a82d-4fd7-8015-2db804aea8e7")
var efiImageSecurityDatabaseGuid = guidFromString("d719b2cb-3d3a-4596-a3bc-dad00e67656f")
var efiGlobalVariable = guidFromString("8be4df61-93ca-11d2-aa0d-00e098032b8c")

// TestSignatureList -
//
//	The bytes written with siglist.Write are identical to the bytes written to dbpem-cli.esl
//	   cert-to-efi-sig-list -g "326aa6de-a82d-4fd7-8015-2db804aea8e7" uefi-db/cert.pem dbpem-cli.esl
func TestSignatureList(t *testing.T) {
	cert, err := CertFromPem(uefiDBPEM)
	if err != nil {
		t.Fail()
	}

	sigdata := efi.SignatureData{Owner: puzzleDbGuid, Data: cert.Raw}
	siglist := efi.SignatureList{Type: efi.CertX509Guid,
		Signatures: []*efi.SignatureData{&sigdata}}

	var b bytes.Buffer
	if err := siglist.Write(&b); err != nil {
		t.Error(err)
	}

	// needs more testing here.
}

func TestLoadDataDir(t *testing.T) {
	cert, err := CertFromPem(uefiDBPEM)
	if err != nil {
		t.Fail()
	}

	tmpd, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal("failed creation of tempdir")
	}
	defer os.RemoveAll(tmpd)

	err = ioutil.WriteFile(filepath.Join(tmpd, "guid"),
		[]byte(puzzleDbGuid.String()+"\n"), 0644)
	if err != nil {
		t.Fatal("failed to write guid to file")
	}

	err = ioutil.WriteFile(filepath.Join(tmpd, "cert.pem"), uefiDBPEM, 0644)
	if err != nil {
		t.Fatalf("Failed writing pem file: %v", err)
	}

	sigdatam, err := LoadSignatureDataDirs(tmpd)
	if err != nil {
		t.Errorf("Call LoadSignatureDataDir failed: %v", err)
	}

	if len(sigdatam) != 1 {
		t.Errorf("Found %d entries expected 1", len(sigdatam))
	}

	sigdata := sigdatam[0]

	if sigdata.Owner != puzzleDbGuid {
		t.Errorf("Owner bad. Expected %s found %s", sigdata.Owner, puzzleDbGuid)
	}

	if !bytes.Equal(sigdata.Data, cert.Raw) {
		t.Errorf("Data bad. Found (len=%d) != Expected (len=%d)", len(sigdata.Data), len(cert.Raw))
	}

}

func TestReadWriteCert(t *testing.T) {
	cert, err := CertFromPem(uefiDBPEM)
	if err != nil {
		t.Fail()
	}

	pem, err := PemFromCert(cert)
	if err != nil {
		t.Fail()
	}

	if bytes.Compare(pem, uefiDBPEM) != 0 {
		fmt.Printf("|%s|\n\n", uefiDBPEM)
		fmt.Printf("|%s|\n\n", pem)
		t.Errorf("differed.")
	}

}
