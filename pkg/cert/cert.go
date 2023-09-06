package cert

import (
	"bytes"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"

	efi "github.com/canonical/go-efilib"
)

func CertFromPemFile(path string) (*x509.Certificate, error) {
	pemData, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return CertFromPem(pemData)
}

func CertFromPem(data []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("No PEM block found")
	}
	return x509.ParseCertificate(block.Bytes)
}

func PemFromCert(cert *x509.Certificate) ([]byte, error) {
	var b bytes.Buffer
	if err := pem.Encode(&b, &pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}); err != nil {
		return b.Bytes(), err
	}
	return b.Bytes(), nil
}

func KeyFromPem(data []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("No PEM block found")
	}

	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse key: %w", err)
	}

	if pkey, ok := key.(*rsa.PrivateKey); ok {
		return pkey, nil
	}

	return nil, fmt.Errorf("parsed key was not rsa.PrivateKey")
}

func KeyFromPemFile(path string) (*rsa.PrivateKey, error) {
	pemData, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return KeyFromPem(pemData)
}

func GUIDFromFile(path string) (efi.GUID, error) {
	empty := efi.GUID{}
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return empty, err
	}
	return efi.DecodeGUIDString(strings.TrimRight(string(content), "\n"))
}

func LoadSignatureDataDir(dirPath string) (*efi.SignatureData, error) {
	cert, err := CertFromPemFile(filepath.Join(dirPath, "cert.pem"))
	if err != nil {
		return nil, err
	}

	return &efi.SignatureData{Owner: efi.GUID{}, Data: cert.Raw}, nil
}

func LoadSignatureDataDirs(dirPaths ...string) ([]*efi.SignatureData, error) {
	sigs := []*efi.SignatureData{}
	for _, d := range dirPaths {
		curData, err := LoadSignatureDataDir(d)
		if err != nil {
			return sigs, err
		}
		sigs = append(sigs, curData)
	}
	return sigs, nil
}

// NewEFISignatureDatabase - return an efi.SignatureDatabase containing
// all of the provided SignatureData.
//
// This SignatureDatabase is the same as you would get with:
//
//	cert-to-efi-sig-list -g X x.pem
//	cert-to-efi-sig-list -g Y y.pem
//	...
//	cat x.pem y.pem ... > my.esl
//
// SignatureDatabase is just a slice of SignatureList
// SignatureList has multiple SignatureData in .Signatures
//   - each of its Signatures must be the same size
//   - efi.CertX509Guid is the Type that is used for shim db
//
// SignatureData is a single guid + cert
func NewEFISignatureDatabase(sigDatam []*efi.SignatureData) efi.SignatureDatabase {
	sigdb := efi.SignatureDatabase{}
	for _, sigdata := range sigDatam {
		sigdb = append(sigdb,
			&efi.SignatureList{
				Type:       efi.CertX509Guid,
				Signatures: []*efi.SignatureData{sigdata},
			},
		)
	}
	return sigdb
}
