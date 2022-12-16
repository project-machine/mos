package mosconfig

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// Check that the product cert was signed by the global puzzleos cert
// This version can be used by outside callers, like atomix extract-soci
// Note that this version does not verify product pid.
func VerifyCert(cert []byte, caPath string) error {
	paths := []string{
		"/factory/secure/manifestCA.pem",
		"/factory/secure/layerCA.pem",
		"/manifestCA.pem",
		"/layerCA.pem",
	}
	if caPath != "" {
		paths = append(paths, caPath)
	}

	var rootBytes []byte
	var err error
	for _, p := range paths {
		rootBytes, err = os.ReadFile(p)
		if err == nil || !os.IsNotExist(err) {
			break
		}

	}
	if err != nil {
		return fmt.Errorf("Failed reading OCI signing CA: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(rootBytes) {
		return fmt.Errorf("Failed adding cert from OCI signing CA")
	}

	block, _ := pem.Decode(cert)
	if block == nil {
		return fmt.Errorf("Failed to parse manifest-signing certificate PEM: %w", err)
	}
	parsedCert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("Failed reading certificate from manifest: %w", err)
	}

	opts := x509.VerifyOptions{
		Roots:     pool,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	}
	_, err = parsedCert.Verify(opts)
	if err != nil {
		return fmt.Errorf("OCI signing certificate verification failed: %w", err)
	}
	return nil
}

func VerifySignature(manifestPath, sigPath, certPath, caPath string) error {
	xtract := []string{"openssl", "x509", "-in", certPath, "-pubkey", "-noout"}
	cert, err := os.ReadFile(certPath)
	if err != nil {
		return fmt.Errorf("Failed reading manifest cert: %w", err)
	}
	pubout, _, err := RunWithStdall(string(cert), xtract...)

	tmpd, err := os.MkdirTemp("", "pubkey")
	if err != nil {
		return fmt.Errorf("Failed creating a tempdir: %w", err)
	}
	defer os.RemoveAll(tmpd)
	keyPath := filepath.Join(tmpd, "pub.key")
	err = os.WriteFile(keyPath, []byte(pubout), 0600)
	if err != nil {
		return fmt.Errorf("Failed writing out public key: %w", err)
	}

	err = VerifyCert(cert, caPath)
	if err != nil {
		return fmt.Errorf("Manifest certificate does not match the CA: %w", err)
	}


	cmd := []string{"openssl", "dgst", "-sha256", "-verify", keyPath,
		"-signature", sigPath, manifestPath}
	if err = LogCommand(cmd...); err != nil {
		return fmt.Errorf("Failed verifying manifest signature: %w", err)
	}
	return nil
}

// Only used during first install.  Create a new $config/manifest.git/
func initManifest(manifestPath, manifestCert, configPath string) error {
	dir := filepath.Join(configPath, "manifest.git")
	if PathExists(dir) {
		return fmt.Errorf("manifest already exists, chickening out!")
	}

	r, err := git.PlainInit(dir, false)
	fmt.Printf("r %#v err %v\n", r, err)
	testBranch1 := &config.Branch{
		Name:   "main",
		Remote: "origin",
		Merge:  "refs/heads/main",
	}
	err = r.CreateBranch(testBranch1)
	fmt.Printf("err %v\n", err)

	w, err := r.Worktree()
	if err != nil {
		return fmt.Errorf("Failed opening git worktree: %w", err)
	}

	caCert := filepath.Join(configPath, "manifestCA.pem")
	err = VerifySignature(manifestPath, manifestPath + ".signed", manifestCert, caCert)
	if err != nil {
		return err
	}

	dest := filepath.Join(dir, "manifest.yaml.signed")
	err = CopyFileBits(manifestPath + ".signed", dest)
	if err != nil {
		return fmt.Errorf("Failed copying install manifest: %w", err)
	}
	_, err = w.Add("manifest.yaml.signed")
	if err != nil {
		return fmt.Errorf("Git file add for manifest signature failed: %w", err)
	}

	dest = filepath.Join(dir, "manifest.yaml")
	err = CopyFileBits(manifestPath, dest)
	if err != nil {
		return fmt.Errorf("Failed copying install manifest: %w", err)
	}
	_, err = w.Add("manifest.yaml")
	if err != nil {
		return fmt.Errorf("Git file add for manifest failed: %w", err)
	}

	dest = filepath.Join(dir, "manifestCert.pem")
	err = CopyFileBits(manifestCert, dest)
	if err != nil {
		return fmt.Errorf("Failed copying manifest Cert: %w", err)
	}
	_, err = w.Add("manifestCert.pem")
	if err != nil {
		return fmt.Errorf("Git file add for cert failed: %w", err)
	}
	commitOpts := &git.CommitOptions{
		Author: defaultSignature(),
		Committer: defaultSignature(),
	}
	_, err = w.Commit("Initial commit", commitOpts)
	return nil
}

func defaultSignature() *object.Signature {
	when := time.Now()
	return &object.Signature{
		Name:  "machine",
		Email: "root@machine.local",
		When:  when,
	}
}
