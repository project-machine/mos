package trust

import (
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/apex/log"
)

// This is the tpm2 backend using tpm2-tools.

const (
	optHierarchyOwner    = "--hierarchy=o"
	optHierarchyPlatform = "--hierarchy=p"
	optHierarchyNone     = "--hierarchy=n"
	optInputStdin        = "--input=-"
)

type TrialPolicy bool

const (
	PolicySession TrialPolicy = false
	TrialSession              = true
)

const RootfsPCRFile = "/pcr7.bin"

func readHostPcr7() ([]byte, error) {
	f, err := ioutil.TempFile("/tmp", "pcr")
	if err != nil {
		return []byte{}, err
	}
	name := f.Name()
	f.Close()
	defer os.Remove(name)
	cmd := []string{"tpm2_pcrread", "sha256:7", "-o", name}
	env := []string{"TPM2TOOLS_TCTI=device:/dev/tpm0"}
	err = runEnv(cmd, env)
	if err != nil {
		return []byte{}, err
	}
	contents, err := ioutil.ReadFile(name)
	return contents, err
}

func curPcr7() (string, error) {
	var c []byte
	var err error
	if PathExists(RootfsPCRFile) {
		c, err = ioutil.ReadFile(RootfsPCRFile)
	} else {
		c, err = readHostPcr7()
	}
	if err != nil {
		return "", fmt.Errorf("Error reading host pcr7: %w", err)
	}
	if nativeEndian == binary.LittleEndian {
		for start := 0; start+1 < len(c); start += 2 {
			tmp := c[start]
			c[start] = c[start+1]
			c[start+1] = tmp
		}
	}
	ret := ""
	for _, x := range c {
		ret = ret + fmt.Sprintf("%02x", x)
	}
	return ret, nil
}

func (c *tpm2V3Context) Tpm2FlushContext() {
	if c.sessionFile == "" {
		return
	}
	run("tpm2_flushcontext", c.sessionFile)
	os.Remove(c.sessionFile)
	c.sessionFile = ""
	if c.Keyctx != "" {
		os.Remove(c.Keyctx)
		c.Keyctx = ""
	}
}

func (c *tpm2V3Context) TempFile() *os.File {
	f, err := ioutil.TempFile(c.tmpDir, "")
	if err != nil {
		log.Fatalf("Failed to create a tmpfile in %s", c.tmpDir)
	}
	return f
}

func (c *tpm2V3Context) Tpm2LoadExternal(pubkeyPath string) error {
	f := c.TempFile()
	c.pubkeyContext = f.Name()
	f.Close()

	pkf := c.TempFile()
	c.pubkeyName = pkf.Name()
	pkf.Close()

	return run("tpm2_loadexternal", optHierarchyOwner,
		"--key-algorithm=rsa", "--public="+pubkeyPath, "--key-context="+c.pubkeyContext, "--name="+c.pubkeyName)
}

func Tpm2NVIndexLength(nvindex NVIndex) (int, error) {
	log.Debugf("Tpm2NVIndexLength(nvindex=%s)\n", nvindex.String())
	stdout, stderr, rc := runCapture("tpm2_nvreadpublic", nvindex.String())
	if rc != 0 {
		return 0, fmt.Errorf("Reading index %s failed:\nstderr: %s\nstdout: %s\n", nvindex, stderr, stdout)
	}
	// 0x1500030:
	//   name: 000b26e01e73e4f489024a06a3687b4621e4d4f2ce865f78d656d8b6c2d06b322f86
	//   hash algorithm:
	//     friendly: sha256
	//     value: 0xB
	//   attributes:
	//     friendly: ownerwrite|ownerread|policyread|written
	//     value: 0x2000A20
	//   size: 40
	//   authorization policy: 56E6476B16D9833592FF236C6E35AE7B7991535DBC83CEE6B30D404E246C29A6
	var re = regexp.MustCompile(`(?m)size:\s(?P<Size>\d+)`)

	matches := re.FindAll(stdout, -1)
	if len(matches) != 1 {
		return 0, fmt.Errorf("Didn't find size field in stdout: %s\n", stdout)
	}

	var size int
	_, err := fmt.Sscanf(string(matches[0]), "size: %d", &size)
	if err != nil {
		return 0, fmt.Errorf("Failed to parse size field from: %s\n", matches[0])
	}

	return size, nil
}
func (c *tpm2V3Context) Tpm2CreatePrimary() error {
	log.Debugf("Tpm2CreatePrimary")
	if c.Keyctx != "" {
		log.Debugf("Tpm2CreatePrimary: a primary context already exists (%s), reusing it", c.Keyctx)
		return nil
	}

	f := c.TempFile()
	fname := f.Name()
	f.Close()

	cmd := []string{"tpm2_createprimary", "--key-context=" + fname}
	if c.adminPwd != "" { // provisioning
		cmd = append(cmd, optHierarchyOwner, "--hierarchy-auth="+c.adminPwd)
	} else {
		// reading
		cmd = append(cmd, optHierarchyNone)
	}

	if err := run(cmd...); err != nil {
		return fmt.Errorf("Error creating primary: %w", err)
	}
	c.Keyctx = fname
	return nil
}

func (c *tpm2V3Context) Tpm2StartSession(isTrial TrialPolicy) error {
	if !isTrial {
		if err := c.Tpm2CreatePrimary(); err != nil {
			return fmt.Errorf("Failed creating primary: %w", err)
		}
	} else {
		c.Keyctx = ""
	}

	f := c.TempFile()
	c.sessionFile = f.Name()
	f.Close()

	cmd := []string{
		"tpm2_startauthsession", "--session=" + c.sessionFile,
	}

	if c.Keyctx != "" {
		cmd = append(cmd, "--key-context="+c.Keyctx)
	}
	if !isTrial {
		cmd = append(cmd, "--policy-session")
	}

	return run(cmd...)
}

func (c *tpm2V3Context) Tpm2PolicyPCR(pcrs string) error {
	return run("tpm2_policypcr", "--session="+c.sessionFile, "--pcr-list="+pcrs)
}

func (c *tpm2V3Context) Tpm2Read(nvindex NVIndex, size int) (string, error) {
	cmd := []string{
		"tpm2_nvread",
		optHierarchyOwner,
		fmt.Sprintf("--size=%d", size),
		nvindex.String(),
	}
	if c.adminPwd != "" {
		cmd = append(cmd, "--auth="+c.adminPwd)
	} else {
		cmd = append(cmd, "--auth=session:"+c.sessionFile)
	}

	stdout, stderr, rc := runCapture(cmd...)
	if rc != 0 {
		return "", fmt.Errorf("Reading %d bytes at index %s failed:\nstderr: %s\nstdout: %s\n",
			size, nvindex, stderr, stdout)
	}
	return string(stdout), nil
}

func (c *tpm2V3Context) Tpm2NVWriteAsAdmin(nvindex NVIndex, towrite string) error {
	cmd := []string{"tpm2_nvwrite", optHierarchyOwner, "--auth=" + c.adminPwd, optInputStdin, nvindex.String()}
	stdout, stderr, rc := runCaptureStdin(towrite, cmd...)
	if rc != 0 {
		return fmt.Errorf("Failed running %s [%d]\nError: %s\nOutput: %s\n", cmd, rc, stderr, stdout)
	}
	return nil
}

func (c *tpm2V3Context) Tpm2NVWriteWithPolicy(nvindex NVIndex, towrite string) error {
	signedPolicyPath := filepath.Join(c.dataDir, "tpm_luks.policy.signed")

	pubkeyPath := c.Pubkeypath("luks")
	err := c.Tpm2LoadExternal(pubkeyPath)
	if err != nil {
		return fmt.Errorf("Failed loading public key: %s: %w", pubkeyPath, err)
	}

	err = c.Tpm2StartSession(PolicySession)
	if err != nil {
		return fmt.Errorf("Failed creating auth session: %w", err)
	}
	defer c.Tpm2FlushContext()

	err = c.Tpm2PolicyPCR(TPM_PCRS_DEF)
	if err != nil {
		return fmt.Errorf("Failed to create PCR Policy event with TPM: %w", err)
	}

	policyVersionSize := 4
	policyVersion, err := Tpm2Read(TPM2IndexEAVersion, policyVersionSize)
	if err != nil {
		return fmt.Errorf("Failed to read PolicyVersion: %w", err)
	}

	policyDigest, err := c.Tpm2PolicyNV(policyVersion)
	if err != nil {
		return fmt.Errorf("The policy version specified does not match contents of TPM NV Index: %w", err)
	}

	ticket, err := c.Tpm2VerifySignature(c.pubkeyContext, policyDigest, signedPolicyPath)
	if err != nil {
		return fmt.Errorf("Failed to verify signature on EA Policy: %s: %w", signedPolicyPath, err)
	}

	// tpm2_policyauthorize
	_, err = c.Tpm2PolicyAuthorizeTicket(policyDigest, ticket)
	if err != nil {
		return fmt.Errorf("Failed to Authorize the EA Policy, invalid signature on the policy digest: %w", err)
	}

	cmd := []string{
		"tpm2_nvwrite",
		fmt.Sprintf("--auth=session:%s", c.sessionFile),
		optInputStdin,
		nvindex.String(),
	}
	stdout, stderr, rc := runCaptureStdin(towrite, cmd...)
	if rc != 0 {
		return fmt.Errorf("Failed running %s [%d]\nError: %s\nOutput: %s\n", cmd, rc, stderr, stdout)
	}

	return nil
}

func (c *tpm2V3Context) Tpm2PolicyNV(towrite string) (string, error) {
	f := c.TempFile()
	fname := f.Name()
	f.Close()

	cmd := []string{"tpm2_policynv", "--session=" + c.sessionFile, optInputStdin, TPM2IndexEAVersion.String(), "eq", "--policy=" + fname}
	stdout, stderr, rc := runCaptureStdin(towrite, cmd...)
	if rc != 0 {
		return "", fmt.Errorf("Failed running %s [%d]\nError: %s\nOutput: %s\n", cmd, rc, stderr, stdout)
	}
	return fname, nil
}

func Tpm2Clear() error {
	// Note: long flag for -c (--auth-hierarchy) does not work with tpm2-tools 4.1.1
	err := run("tpm2_clear", "-c", "p")
	if err != nil {
		err = run("tpm2_clear")
		if err != nil {
			return fmt.Errorf("Error runnign tpm2_clear: %w", err)
		}
	}
	err = run("tpm2_dictionarylockout", "--setup-parameters", "--lockout-recovery-time=120", "--max-tries=4294967295", "--clear-lockout")
	if err != nil {
		return fmt.Errorf("Error setting lockout parameters: %w", err)
	}
	return nil
}

// Write a value which is publically readable but only writeable with tpm admin pass
func (c *tpm2V3Context) StorePublic(idx NVIndex, value string) error {
	attributes := "ownerwrite|ownerread|authread"
	err := c.Tpm2NVDefine("", attributes, idx, len(value))
	if err != nil {
		return err
	}

	return c.Tpm2NVWriteAsAdmin(idx, value)
}

func getTpmBufsize() (int, error) {
	out, rc := RunCommandWithRc("tpm2_getcap", "properties-fixed")
	if rc != 0 {
		return 0, fmt.Errorf("error %d", rc)
	}
	inSection := false
	for _, line := range strings.Split(string(out), "\n") {
		if !inSection {
			if strings.HasPrefix(line, "TPM2_PT_NV_BUFFER_MAX:") {
				inSection = true
			}
			continue
		}
		if line[0] != ' ' {
			return 0, fmt.Errorf("No TPM2_PT_NV_BUFFER_MAX value found")
		}
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "raw:") {
			continue
		}
		fields := strings.Split(line, ":")
		if len(fields) != 2 {
			continue
		}
		word := strings.TrimSpace(fields[1])
		v, err := strconv.ParseInt(word, 0, 32)
		if err != nil {
			return -1, fmt.Errorf("strconv on %v returned error: %w", line[4:], err)
		}
		return int(v), nil
	}
	return 0, fmt.Errorf("No TPM2_PT_NV_BUFFER_MAX found")
}

func (c *tpm2V3Context) Tpm2PolicyAuthorize() (string, error) {
	f := c.TempFile()
	digestFile := f.Name()
	f.Close()

	cmd := []string{"tpm2_policyauthorize",
		"--session=" + c.sessionFile,
		"--name=" + c.pubkeyName,
		"--policy=" + digestFile}
	return digestFile, run(cmd...)
}

func (c *tpm2V3Context) Tpm2PolicyAuthorizeTicket(policyDigest, ticketFile string) (string, error) {
	log.Debugf("Tpm2PolicyAuthorizeTicket(session=%s policyDigest=%s ticketFile=%s)\n", c.sessionFile, policyDigest, ticketFile)
	f := c.TempFile()
	digestFile := f.Name()
	f.Close()

	cmd := []string{"tpm2_policyauthorize", "-S", c.sessionFile, "-i", policyDigest, "-n", c.pubkeyName, "-t", ticketFile}
	return digestFile, run(cmd...)
}

func Tpm2Read(nvindex NVIndex, size int) (string, error) {
	log.Debugf("Tpm2Read(nvindex=%s size=%d)\n", nvindex.String(), size)
	stdout, stderr, rc := RunCommandWithOutputErrorRc("tpm2_nvread", "-s", fmt.Sprintf("%d", size), nvindex.String())
	if rc != 0 {
		return "", fmt.Errorf("Reading %d bytes at index %s failed:\nstderr: %s\nstdout: %s\n",
			size, nvindex, stderr, stdout)
	}
	return string(stdout), nil
}

func (c *tpm2V3Context) Tpm2VerifySignature(pubkeyContextFile, digestFile, signatureFile string) (string, error) {
	log.Debugf("Tpm2VerifySignature(pubkeyContext=%s digestFile=%s sigFile=%s)\n", pubkeyContextFile, digestFile, signatureFile)
	scheme := "rsassa"
	hashAlgo := "sha256"

	f := c.TempFile()
	ticketFile := f.Name()
	f.Close()

	cmd := []string{"tpm2_verifysignature", "-c", pubkeyContextFile, "-f", scheme, "-g", hashAlgo, "-m", digestFile, "-s", signatureFile, "-t", ticketFile}
	return ticketFile, run(cmd...)
}

func (c *tpm2V3Context) Tpm2ReadSession(nvindex NVIndex, offset int, size int) (string, error) {
	log.Debugf("Tpm2ReadSession(session=%s nvindex=%s size=%d)\n", c.sessionFile, nvindex.String(), size)
	cmd := []string{
		"tpm2_nvread",
		fmt.Sprintf("--auth=session:%s", c.sessionFile),
		fmt.Sprintf("--size=%d", size),
		fmt.Sprintf("--offset=%d", offset),
		nvindex.String(),
	}

	stdout, stderr, rc := RunCommandWithOutputErrorRc(cmd...)
	if rc != 0 {
		return "", fmt.Errorf("Reading %d bytes at index %s failed:\nstderr: %s\nstdout: %s\n",
			size, nvindex, stderr, stdout)
	}
	return string(stdout), nil
}

func (c *tpm2V3Context) ReadSecretPiece(idx NVIndex, signedPolicyPath string, offset int, size int) (string, error) {
	err := c.Tpm2StartSession(PolicySession)
	if err != nil {
		return "", fmt.Errorf("Failed creating auth session: %w", err)
	}
	defer c.Tpm2FlushContext()

	err = c.Tpm2PolicyPCR(TPM_PCRS_DEF)
	if err != nil {
		return "", fmt.Errorf("Failed to create PCR Policy event with TPM: %w", err)
	}

	policyVersionSize := 4
	policyVersion, err := Tpm2Read(TPM2IndexEAVersion, policyVersionSize)
	if err != nil {
		return "", fmt.Errorf("Failed to read PolicyVersion: %w", err)
	}

	log.Debugf("tpm2V3Context.ReadSecretPiece() PolicyNVDigest\n")
	policyDigest, err := c.Tpm2PolicyNV(policyVersion)
	if err != nil {
		return "", fmt.Errorf("The policy version specified does not match contents of TPM NV Index: %w", err)
	}

	log.Debugf("tpm2V3Context.ReadSecretPiece() VerifySignature\n")
	ticket, err := c.Tpm2VerifySignature(c.pubkeyContext, policyDigest, signedPolicyPath)
	if err != nil {
		return "", fmt.Errorf("Failed to verify signature on EA Policy: %s: %w", signedPolicyPath, err)
	}

	// tpm2_policyauthorize
	log.Debugf("tpm2V3Context.ReadSecretPiece() PolicyAuthorize\n")
	_, err = c.Tpm2PolicyAuthorizeTicket(policyDigest, ticket)
	if err != nil {
		return "", fmt.Errorf("Failed to Authorize the EA Policy, invalid signature on the policy digest: %w", err)
	}

	// tpm2_nvread
	log.Debugf("tpm2V3Context.ReadSecretPiece() ReadSession\n")
	secret, err := c.Tpm2ReadSession(idx, offset, size)
	if err != nil {
		return "", fmt.Errorf("Failed to read Secret from TPM: %w", err)
	}

	return secret, nil
}

func (c *tpm2V3Context) ReadSecret(idx NVIndex, signedPolicyPath string) (string, error) {
	log.Debugf("tpm2V3Context.ReadSecret(signed policy=%s)\n", signedPolicyPath)
	bufsize, err := getTpmBufsize()
	if err != nil {
		return "", err
	}
	secretLength, err := Tpm2NVIndexLength(idx)
	if err != nil {
		return "", fmt.Errorf("Failed to obtain length of NV index %s: %w", idx, err)
	}

	log.Debugf("tpm2V3Context.ReadSecret() loadExternal\n")
	pubkeyPath := c.Pubkeypath("luks")
	err = c.Tpm2LoadExternal(pubkeyPath)
	if err != nil {
		return "", fmt.Errorf("Failed loading public key: %s: %w", pubkeyPath, err)
	}

	log.Debugf("reading %s, got bufsize %d secretlength %d", idx, bufsize, secretLength)
	whole := ""
	offset := 0
	for secretLength > 0 {
		copySize := secretLength
		if copySize > bufsize {
			copySize = bufsize
		}
		log.Debugf("reading %d bytes at offset %d", copySize, offset)
		piece, err := c.ReadSecretPiece(idx, signedPolicyPath, offset, copySize)
		if err != nil {
			return "", fmt.Errorf("Reading offset %d size %d of %s returned error : %w", offset, copySize, idx, err)
		}
		whole = whole + piece
		secretLength -= copySize
		offset += copySize
	}

	return whole, nil
}

func (c *tpm2V3Context) Tpm2NVDefine(digestfile string, attr string, index NVIndex, l int) error {
	length := fmt.Sprintf("%d", l)
	cmd := []string{"tpm2_nvdefine", "--attributes=" + attr, "--hierarchy-auth=" + c.adminPwd}
	if digestfile != "" {
		cmd = append(cmd, "--policy="+digestfile)
	}
	cmd = append(cmd, "--size="+length, index.String())
	return run(cmd...)
}

// Store the TPM password.  This is a lot more than that, though:
// 1. load the public signing key into the TPM
// 2. load the EA policy to protect the password
func (c *tpm2V3Context) StoreAdminPassword() error {
	contexts := []string{"owner", "endorsement", "lockout"}
	for _, context := range contexts {
		err := run("tpm2_changeauth", "--object-context="+context, c.adminPwd)
		if err != nil {
			return err
		}
	}

	pubkeyPath := c.Pubkeypath("tpmpass")
	err := c.Tpm2LoadExternal(pubkeyPath)
	if err != nil {
		return fmt.Errorf("Failed loading tpm-passwd policy public key: %w", err)
	}

	err = c.Tpm2StartSession(TrialSession)
	if err != nil {
		return fmt.Errorf("Failed creating trial auth session: %w", err)
	}
	policyDigestFile, err := c.Tpm2PolicyAuthorize()
	if err != nil {
		return fmt.Errorf("Failed authorizing PCR policy: %w", err)
	}

	attributes := "ownerwrite|ownerread|policyread"
	err = c.Tpm2NVDefine(policyDigestFile, attributes, TPM2IndexPassword, len(c.adminPwd))
	if err != nil {
		return fmt.Errorf("Failed defining NV: %w", err)
	}
	c.Tpm2FlushContext()

	err = c.Tpm2NVWriteAsAdmin(TPM2IndexPassword, c.adminPwd)
	if err != nil {
		return fmt.Errorf("Failed writing TPM passphrase to TPM: %w", err)
	}

	return nil
}

// echo atomix | sha256sum
const atxSha = "b7135cbb321a66fa848b07288bd008b89bd5b7496c4569c5e1a4efd5f7c8e0a7"

func (t *tpm2V3Context) extendPCR7() error {
	cmd := []string{"tpm2_pcrextend", "7:sha256=" + atxSha}
	return run(cmd...)
}
