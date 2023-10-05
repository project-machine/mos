package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/project-machine/mos/pkg/trust"
	"github.com/project-machine/mos/pkg/utils"
)

func getSudiDir() (string, error) {
	dataDir, err := utils.UserDataDir()
	if err != nil {
		return "", err
	}
	sudiPath := filepath.Join(dataDir, "machine", "trust")
	return sudiPath, os.MkdirAll(sudiPath, 0755)
}

func KeysetExists(keysetname string) bool {
	mosKeyPath, err := utils.GetMosKeyPath()
	if err != nil {
		return false
	}
	keysetPath := filepath.Join(mosKeyPath, keysetname)
	if utils.PathExists(keysetPath) {
		return true
	} else {
		return false
	}
}

// SignCert creates a CA signed certificate and keypair in destdir
func SignCert(template, CAcert *x509.Certificate, CAkey any, destdir string) error {
	// Check if credentials already exist
	if utils.PathExists(filepath.Join(destdir, "privkey.pem")) {
		return fmt.Errorf("credentials already exist in %s", destdir)
	}

	// Save private key
	keyPEM, err := os.OpenFile(
		filepath.Join(destdir, "privkey.pem"),
		os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer keyPEM.Close()

	certPEM, err := os.OpenFile(
		filepath.Join(destdir, "cert.pem"),
		os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0640)
	if err != nil {
		os.Remove(keyPEM.Name())
		return err
	}
	defer certPEM.Close()

	if err := signCertToFiles(template, CAcert, CAkey, certPEM, keyPEM); err != nil {
		os.Remove(keyPEM.Name())
		os.Remove(certPEM.Name())
		return err
	}

	return nil
}

func signCertToFiles(template, CAcert *x509.Certificate, CAkey any,
	certWriter io.Writer, keyWriter io.Writer) error {
	// Generate a keypair
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}

	err = privKey.Validate()
	if err != nil {
		return err
	}

	// Additional info to add to certificate template
	serialNo, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return err
	}

	// SubjectKeyID is sha1 hash of the public key
	pubKey := privKey.PublicKey
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(&pubKey)
	if err != nil {
		return err
	}
	subjectKeyID := sha1.Sum(publicKeyBytes)

	template.SerialNumber = serialNo
	template.SubjectKeyId = subjectKeyID[:]

	signedCert, err := x509.CreateCertificate(rand.Reader, template, CAcert, &pubKey, CAkey)
	if err != nil {
		return err
	}

	pkcs8, err := x509.MarshalPKCS8PrivateKey(privKey)
	if err != nil {
		return err
	}
	err = pem.Encode(keyWriter, &pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})
	if err != nil {
		return err
	}

	err = pem.Encode(certWriter, &pem.Block{Type: "CERTIFICATE", Bytes: signedCert})
	if err != nil {
		return err
	}

	return nil
}

func readCertificateFromFile(CApath string) (*x509.Certificate, error) {
	// Get the rootCA cert & privKey
	certFile, err := os.ReadFile(CApath)
	if err != nil {
		return nil, err
	}
	pemBlock, _ := pem.Decode(certFile)
	if pemBlock == nil {
		return nil, errors.New("pem.Decode cert failed")
	}
	return x509.ParseCertificate(pemBlock.Bytes)
}

func readPrivKeyFromFile(keypath string) (any, error) {
	keyFile, err := os.ReadFile(keypath)
	if err != nil {
		return nil, err
	}
	pemBlock, _ := pem.Decode(keyFile)
	if pemBlock == nil {
		return nil, errors.New("pem.Decode cert failed")
	}
	return x509.ParsePKCS8PrivateKey(pemBlock.Bytes)
}

func getCA(CAname, keysetName string) (*x509.Certificate, any, error) {
	// locate the keyset
	keysetPath, err := utils.GetMosKeyPath()
	if err != nil {
		return nil, nil, err
	}
	keysetPath = filepath.Join(keysetPath, keysetName)
	if !utils.PathExists(keysetPath) {
		return nil, nil, fmt.Errorf("keyset %s, does not exist", keysetName)
	}

	CAcert, err := readCertificateFromFile(filepath.Join(keysetPath, CAname, "cert.pem"))
	// See if the CA exists
	CApath := filepath.Join(keysetPath, CAname)
	if !utils.PathExists(CApath) {
		return nil, nil, fmt.Errorf("%s CA does not exist", CAname)
	}

	// Get the rootCA cert & privKey
	CAkey, err := readPrivKeyFromFile(filepath.Join(keysetPath, CAname, "privkey.pem"))

	return CAcert, CAkey, nil
}

func generateNewUUIDCreds(keysetName, destdir string) error {
	// Create new manifest credentials
	newUUID := uuid.NewString()

	// Create a certificate template
	CN := fmt.Sprintf("manifest PRODUCT:%s", newUUID)
	certTemplate := x509.Certificate{
		Subject: pkix.Name{
			CommonName: CN,
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().AddDate(20, 0, 0),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageCodeSigning},
	}

	// get CA privkey and CA cert
	CAcert, CAprivkey, err := getCA("manifest-ca", keysetName)
	if err != nil {
		return err
	}

	err = SignCert(&certTemplate, CAcert, CAprivkey, destdir)
	if err != nil {
		return err
	}

	err = os.WriteFile(filepath.Join(destdir, "uuid"), []byte(newUUID), 0640)
	if err != nil {
		os.Remove(filepath.Join(destdir, "privkey.pem"))
		os.Remove(filepath.Join(destdir, "cert.pem"))
		return err
	}

	return nil
}

func generaterootCA(destdir string, caTemplate *x509.Certificate, doguid bool) error {
	// Generate keypair
	privkey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}
	err = privkey.Validate()
	if err != nil {
		return err
	}

	// Include a serial number and generate self-signed certificate
	serialNo, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return err
	}
	caTemplate.SerialNumber = serialNo

	rootCA, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &privkey.PublicKey, privkey)
	if err != nil {
		return err
	}
	// Save the private key and cert to the specified directory
	defer func() {
		if err != nil {
			os.Remove(filepath.Join(destdir, "privkey.pem"))
			os.Remove(filepath.Join(destdir, "cert.pem"))
		}
	}()

	keyPEM, err := os.Create(filepath.Join(destdir, "privkey.pem"))
	if err != nil {
		return err
	}
	defer keyPEM.Close()

	pkcs8, err := x509.MarshalPKCS8PrivateKey(privkey)
	if err != nil {
		return err
	}
	err = pem.Encode(keyPEM, &pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})
	if err != nil {
		return err
	}
	err = os.Chmod(filepath.Join(destdir, "privkey.pem"), 0600)
	if err != nil {
		return err
	}

	// Save signed certificate
	certPEM, err := os.Create(filepath.Join(destdir, "cert.pem"))
	if err != nil {
		return err
	}
	defer certPEM.Close()

	err = pem.Encode(certPEM, &pem.Block{Type: "CERTIFICATE", Bytes: rootCA})
	if err != nil {
		return err
	}
	err = os.Chmod(filepath.Join(destdir, "cert.pem"), 0640)
	if err != nil {
		return err
	}

	// Is a guid needed...
	if doguid {
		guid := uuid.NewString()
		err = os.WriteFile(filepath.Join(destdir, "guid"), []byte(guid), 0640)
		if err != nil {
			return err
		}
	}

	return nil
}

// Generates an RSA 2048 keypair, self-signed cert and a guid if specified.
func generateCreds(destdir string, doguid bool, template *x509.Certificate) error {
	// Generate keypair
	privkey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}
	err = privkey.Validate()
	if err != nil {
		return err
	}

	// Add additional info to certificate
	serialNo, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return err
	}
	template.SerialNumber = serialNo
	// SubjectKeyID is sha1 hash of the public key
	pubKey := privkey.PublicKey
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(&pubKey)
	if err != nil {
		return err
	}
	subjectKeyID := sha1.Sum(publicKeyBytes)
	template.SubjectKeyId = subjectKeyID[:]

	newcert, err := x509.CreateCertificate(rand.Reader, template, template, &privkey.PublicKey, privkey)
	if err != nil {
		return err
	}

	// Save the private key and cert to the specified directory
	defer func() {
		if err != nil {
			os.Remove(filepath.Join(destdir, "privkey.pem"))
			os.Remove(filepath.Join(destdir, "cert.pem"))
		}
	}()
	keyPEM, err := os.Create(filepath.Join(destdir, "privkey.pem"))
	if err != nil {
		return err
	}
	defer keyPEM.Close()

	pkcs8, err := x509.MarshalPKCS8PrivateKey(privkey)
	if err != nil {
		return err
	}
	err = pem.Encode(keyPEM, &pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})
	if err != nil {
		return err
	}
	err = os.Chmod(filepath.Join(destdir, "privkey.pem"), 0600)
	if err != nil {
		return err
	}

	// Save signed certificate
	certPEM, err := os.Create(filepath.Join(destdir, "cert.pem"))
	if err != nil {
		return err
	}
	defer certPEM.Close()
	err = pem.Encode(certPEM, &pem.Block{Type: "CERTIFICATE", Bytes: newcert})
	if err != nil {
		return err
	}
	err = os.Chmod(filepath.Join(destdir, "cert.pem"), 0640)
	if err != nil {
		return err
	}

	// Is a guid needed...
	if doguid {
		guid := uuid.NewString()
		err = os.WriteFile(filepath.Join(destdir, "guid"), []byte(guid), 0640)
		if err != nil {
			return err
		}
	}

	return nil
}

// Flips the bits of pcr7val to produce an index for pcr7dara dir.
// Copies the value so we do not alter original.
func createPCR7Index(pcr7Val []byte) (string, error) {
	c := make([]byte, len(pcr7Val))
	copy(c, pcr7Val)
	for start := 0; start+1 < len(c); start += 2 {
		tmp := c[start]
		c[start] = c[start+1]
		c[start+1] = tmp
	}
	encodedStr := hex.EncodeToString(c)
	return encodedStr, nil
}

func extractPubkey(certPath string) (*rsa.PublicKey, error) {
	parsedCert, err := readCertificateFromFile(certPath)
	if err != nil {
		return nil, err
	}
	return parsedCert.PublicKey.(*rsa.PublicKey), nil
}

func savePubkeytoFile(pubkey *rsa.PublicKey, outPath string) error {
	pubkeyPem, err := os.Create(outPath)
	if err != nil {
		return err
	}
	pkix, err := x509.MarshalPKIXPublicKey(pubkey)
	if err != nil {
		return err
	}
	err = pem.Encode(pubkeyPem, &pem.Block{Type: "PUBLIC KEY", Bytes: pkix})
	if err != nil {
		return err
	}
	err = os.Chmod(outPath, 0644)
	if err != nil {
		return err
	}
	return nil
}

type pcr7Data struct {
	limited            []byte
	tpm                []byte
	production         []byte
	passwdPolicyDigest []byte
	luksPolicyDigest   []byte
}

// Create the various signdata artifacts for a keyset and
// add to the pcr7data dir of the keyset.
func addPcr7data(keysetName string, pdata pcr7Data) error {
	var err error
	var notexist = true
	var moskeypath, pcrIndex string
	var tpmpolAdminpubkey, tpmpolLukspubkey *rsa.PublicKey
	var jsonInfo []byte
	type PCR7info struct {
		Key     string `json:"key"`
		KeyType string `json:"key_type"`
		Date    string `json:"est_date"`
		Comment string `json:"comment"`
	}

	// Check inputs
	if keysetName == "" {
		return errors.New("Please specify a keyset name")
	}
	moskeypath, err = utils.GetMosKeyPath()
	if err != nil {
		return err
	}
	keysetPath := filepath.Join(moskeypath, keysetName)
	if !utils.PathExists(keysetPath) {
		return fmt.Errorf("The keyset, %s, does not exist.", keysetName)
	}

	if pdata.limited == nil || pdata.tpm == nil || pdata.production == nil {
		return errors.New("Must specify all 3 pcr7 values: tpm, limited and production")
	}

	if pdata.passwdPolicyDigest == nil || pdata.luksPolicyDigest == nil {
		return errors.New("The passwd policy file is missing.")
	}

	// Create pcr7data directory if it does not exist.
	// Its ok if pcr7data dir already exists. We might be adding additional signdata
	pcr7dataPath := filepath.Join(keysetPath, "pcr7data/policy-2")
	if !utils.PathExists(pcr7dataPath) {
		err = os.MkdirAll(keysetPath, 0750)
		if err != nil {
			return err
		}
	} else {
		notexist = false
	}

	// Dont remove pcr7data dir on error if it already existed.
	defer func() {
		if err != nil && notexist == true {
			os.RemoveAll(filepath.Join(keysetPath, "pcr7data"))
		}
	}()

	// Check to see if public keys already exist for this keyset, if not
	// then extract public keys and save them to pcr7data dir.
	pcr7dataPubkeys := filepath.Join(pcr7dataPath, "pubkeys")
	if !utils.PathExists(pcr7dataPubkeys) {
		if err = utils.EnsureDir(pcr7dataPubkeys); err != nil {
			return errors.New("Failed to create directory for public keys")
		}
	}
	tpmpolAdminpubkey, err = extractPubkey(filepath.Join(keysetPath, "tpmpol-admin/cert.pem"))
	if err != nil {
		return err
	}
	destpath := filepath.Join(pcr7dataPubkeys, fmt.Sprintf("tpmpass-%s.pem", keysetName))
	err = savePubkeytoFile(tpmpolAdminpubkey, destpath)
	if err != nil {
		return err
	}
	tpmpolLukspubkey, err = extractPubkey(filepath.Join(keysetPath, "tpmpol-luks/cert.pem"))
	if err != nil {
		return err
	}

	destpath = filepath.Join(pcr7dataPubkeys, fmt.Sprintf("luks-%s.pem", keysetName))
	err = savePubkeytoFile(tpmpolLukspubkey, destpath)
	if err != nil {
		return err
	}

	// - Generate the index for pcr7-limited.bin.
	//    Add the binary pcr7 values into this index.
	// - Generate the index for pcr7-production.bin.
	//    Add the signed tpm-luks policy into this index.
	// - Generate the index for pcr7-tpm.bin.
	//    Add the signed tpm-passwd policy into this index.
	pcr7 := make(map[string][]byte)
	pcr7["limited"] = pdata.limited
	pcr7["tpm"] = pdata.tpm
	pcr7["production"] = pdata.production

	for pcr, value := range pcr7 {
		// create index used to name the sub-directories under policy-2 directory
		pcrIndex, err = createPCR7Index(value)
		if err != nil {
			return err
		}
		indexdir := filepath.Join(pcr7dataPath, pcrIndex[0:2], pcrIndex[2:])
		if err = utils.EnsureDir(indexdir); err != nil {
			return err
		}

		// create info.json
		jsonFile := filepath.Join(indexdir, "info.json")

		date := time.Now()
		formatted := date.Format("2006-01-02")
		timestamp := strings.ReplaceAll(formatted, "-", "")
		info := &PCR7info{Key: keysetName, KeyType: pcr, Date: timestamp, Comment: "mos" + " " + keysetName}
		jsonInfo, err = json.Marshal(info)
		if err != nil {
			return err
		}
		if err = os.WriteFile(jsonFile, jsonInfo, 0644); err != nil {
			return err
		}

		// write out info
		switch pcr {
		case "limited":
			pcrFile := filepath.Join(indexdir, "pcr_limited.bin")
			if err = os.WriteFile(pcrFile, pdata.limited, 0644); err != nil {
				return err
			}
			pcrFile = filepath.Join(indexdir, "pcr_tpm.bin")
			if err = os.WriteFile(pcrFile, pdata.tpm, 0644); err != nil {
				return err
			}
			pcrFile = filepath.Join(indexdir, "pcr_prod.bin")
			if err = os.WriteFile(pcrFile, pdata.production, 0644); err != nil {
				return err
			}
		case "tpm":
			// Create policy file and Sign the policy
			policyFile := filepath.Join(indexdir, "tpm_passwd.policy.signed")
			if err = os.WriteFile(policyFile, pdata.passwdPolicyDigest, 0644); err != nil {
				return err
			}
			signingKey := filepath.Join(keysetPath, "tpmpol-admin/privkey.pem")
			if err = trust.Sign(policyFile, policyFile, signingKey); err != nil {
				return err
			}
		case "production":
			// Sign the policy
			policyFile := filepath.Join(indexdir, "tpm_luks.policy.signed")
			if err = os.WriteFile(policyFile, pdata.luksPolicyDigest, 0644); err != nil {
				return err
			}
			signingKey := filepath.Join(keysetPath, "tpmpol-luks/privkey.pem")
			if err = trust.Sign(policyFile, policyFile, signingKey); err != nil {
				return err
			}
		default:
			return errors.New("Unrecognized uki key")
		}
	}
	return nil
}
