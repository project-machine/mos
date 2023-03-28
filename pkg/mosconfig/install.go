package mosconfig

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	digest "github.com/opencontainers/go-digest"
	"github.com/opencontainers/umoci"
	"github.com/pkg/errors"
	"github.com/project-machine/trust/pkg/trust"
	"github.com/urfave/cli"
	"github.com/opencontainers/image-spec/specs-go"
	ispec "github.com/opencontainers/image-spec/specs-go/v1"
	"gopkg.in/yaml.v2"

	"stackerbuild.io/stacker/pkg/lib"
)

// InstallSource represents an install file, its signature, and
// certificate for verifying the signature, downloaded from an
// oci repo url and its referrers.
type InstallSource struct {
	Basedir  string
	FilePath string
	CertPath string
	SignPath string
	ocirepo  *DistRepo

	NeedsCleanup bool
}

// cleaning up is only done if we created the tempdir
func (is *InstallSource) Cleanup() {
	if is.NeedsCleanup {
		os.RemoveAll(is.Basedir)
		is.NeedsCleanup = false
	}
}

func (is *InstallSource) FetchFromZot(inUrl string) error {
	dir, err := os.MkdirTemp("", "install")
	if err != nil {
		return err
	}
	is.Basedir = dir
	is.FilePath = filepath.Join(is.Basedir, "install.json") // TODO - switch to json
	is.CertPath = filepath.Join(is.Basedir, "manifestCert.pem")
	is.SignPath = filepath.Join(is.Basedir, "install.json.signed")

	r, err := NewDistRepo(inUrl)
	if err != nil {
		return errors.Wrapf(err, "Error opening OCI repo connection")
	}
	is.ocirepo = r

	url, err := r.openUrl(inUrl)
	if err != nil {
		return errors.Wrapf(err, "Error parsing install manifest url")
	}

	err = r.FetchInstall(url, is.FilePath)
	if err != nil {
		return errors.Wrapf(err, "Error fetching the install manifest")
	}

	err = r.FetchCert(url, is.CertPath)
	if err != nil {
		return errors.Wrapf(err, "Error fetching the certificate")
	}

	err = r.FetchSignature(url, is.SignPath)
	if err != nil {
		return errors.Wrapf(err, "Error fetching the signature")
	}

	is.NeedsCleanup = true

	return nil
}

func InitializeMos(ctx *cli.Context) error {
	storeDir := "/atomfs-store"
	configDir := "/config"
	caPath := "/factory/secure/manifestCA.pem"
	rfs := "/"
	if ctx.IsSet("rfs") {
		rfs = ctx.String("rfs")
		caPath = filepath.Join(rfs, caPath)
		configDir = filepath.Join(rfs, configDir)
		storeDir = filepath.Join(rfs, storeDir)
	}
	if !PathExists(caPath) {
		return errors.Errorf("Install manifest CA (%s) missing", caPath)
	}
	if !PathExists(configDir) {
		return errors.Errorf("Configuration directory (%s) missing", configDir)
	}
	if !PathExists(storeDir) {
		return errors.Errorf("Storage cache dir (%s) missing", storeDir)
	}

	args := ctx.Args()
	if len(args) < 1 {
		return errors.Errorf("An install source is required.\nUsage: mos install [--config-dir /config] [--atomfs-store /atomfs-store] docker://10.0.2.2:5000/mos/install.json:1.0")
	}

	var is InstallSource
	defer is.Cleanup()

	err := is.FetchFromZot(args[0])
	if err != nil {
		return err
	}

	mos, err := NewMos(configDir, storeDir)
	if err != nil {
		return errors.Errorf("Error opening manifest: %w", err)
	}
	defer mos.Close()

	// Well, bit of a chicken and egg problem here.  We parse the configfile
	// first so we can copy all the needed zot images.
	cf, err := simpleParseInstall(is.FilePath)
	if err != nil {
		return errors.Wrapf(err, "Failed parsing install configuration")
	}

	for _, target := range cf.Targets {
		src := fmt.Sprintf("docker://%s/mos:%s", is.ocirepo.addr, dropHashAlg(target.Digest))
		err = mos.storage.ImportTarget(src, &target)
		if err != nil {
			return errors.Wrapf(err, "Failed reading targets while initializing mos")
		}
	}

	if cf.UpdateType == PartialUpdate {
		return errors.Errorf("Cannot install with a partial manifest")
	}

	// Finally set up our manifest store
	// The manifest will be re-read as it is verified.
	err = mos.initManifest(is.FilePath, is.CertPath, caPath, configDir)
	if err != nil {
		return errors.Errorf("Error initializing system manifest: %w", err)
	}

	return nil
}

// PublishManifest is used by mosctl to convert and publish a
// import manifest in yaml format to a install manifest in json
// format, sign it, and post all referenced layers as well as the
// manifest, cert, and signature to the listed repository.
func PublishManifest(ctx *cli.Context) error {
	cert := ctx.String("cert")
	if cert == "" {
		return fmt.Errorf("Certificate filename is required")
	}
	key := ctx.String("key")
	if key == "" {
		return fmt.Errorf("Key filename is required")
	}
	repo := ctx.String("repo")
	if cert == "" {
		return fmt.Errorf("Repo is required")
	}
	destpath := ctx.String("name")
	if cert == "" {
		return fmt.Errorf("Repo is required")
	}
	args := ctx.Args()
	if len(args) != 1 {
		return fmt.Errorf("file is a required positional argument")
	}
	infile := args[0]

	b, err := os.ReadFile(infile)
	if err != nil {
		return errors.Wrapf(err, "Error reading %s", infile)
	}

	var imports ImportFile
	err = yaml.Unmarshal(b, &imports)
	if err != nil {
		return errors.Wrapf(err, "Error parsing %s", infile)
	}

	if imports.Version != CurrentInstallFileVersion {
		return errors.Errorf("Unknown import file version: %d (I know about %d)", imports.Version, CurrentInstallFileVersion)
	}

	install := InstallFile{
		Version: imports.Version,
		Product: imports.Product,
		UpdateType: imports.UpdateType,
	}

	// Copy each of the targets to specified oci repo,
	// verify digest and size, and append them to the install
	// manifest's list.
	for _, t := range imports.Targets {
		digest, size, err := getSizeDigest(t.Source)
		if err != nil {
			return errors.Wrapf(err, "Failed checking %s", t.Source)
		}
		if t.Digest != digest {
			return errors.Errorf("Digest (%s) specified for %s does not match remote image's (%s)", t.Digest, t.Source, digest)
		}
		if t.Size != size {
			return errors.Errorf("Size (%d) specified for %s does not match remote image's (%s)", t.Size, t.Source, size)
		}

		dest := "docker://" + repo + "/mos:" + dropHashAlg(digest)
		copyOpts := lib.ImageCopyOpts{
			Src: t.Source,
			Dest:        dest,
			Progress:    os.Stdout,
			SrcSkipTLS:  true,
			DestSkipTLS: true,
		}
		if err := lib.ImageCopy(copyOpts); err != nil {
			return errors.Wrapf(err, "Failed copying %s to %s", t.Source, dest)
		}
		install.Targets = append(install.Targets, Target{
			ServiceName: t.ServiceName,
			Version:     t.Version,
			ServiceType: t.ServiceType,
			Network:     t.Network,
			NSGroup:     t.NSGroup,
			Digest:      digest,
			Size:        size},
		)
	}

	workdir, err := os.MkdirTemp("", "manifest")
	if err != nil {
		return errors.Wrapf(err, "Failed creating tempdir")
	}
	defer os.RemoveAll(workdir)

	b, err = json.Marshal(&install)
	if err != nil {
		return errors.Wrapf(err, "Failed encoding the install.json")
	}
	filePath := filepath.Join(workdir, "install.json")
	if err := os.WriteFile(filePath, b, 0644); err != nil {
		return errors.Wrapf(err, "Failed opening %s for writing", filePath)
	}

	signPath := filepath.Join(workdir, "install.json.signed")
	if err = trust.Sign(filePath, signPath, key); err != nil {
		return errors.Wrapf(err, "Failed signing file")
	}

	dest := repo + "/" + destpath
	mDigest, mSize, err := PostManifest(filePath, dest)
	if err != nil {
		return errors.Wrapf(err, "Failed writing install.json to %s", dest)
	}
	if err = PostArtifact(mDigest, mSize, cert, "vnd.machine.pubkeycrt", dest); err != nil {
		return errors.Wrapf(err, "Failed writing certificate to %s", dest)
	}
	if err = PostArtifact(mDigest, mSize, signPath, "vnd.machine.signature", dest); err != nil {
		return errors.Wrapf(err, "Failed writing signature to %s", dest)
	}

	return nil
}

func getSizeDigestOCI(inUrl string) (string, int64, error) {
	split := strings.SplitN(inUrl, ":", 3)
	if len(split) != 3 {
		return "", 0, errors.Errorf("Bad oci url: %s", inUrl)
	}
	ocidir := split[1]
	image := split[2]
	oci, err := umoci.OpenLayout(ocidir)
	if err != nil {
		return "", 0, errors.Wrapf(err, "Failed opening oci layout at %q", ocidir)
	}
	dp, err := oci.ResolveReference(context.Background(), image)
	if err != nil {
		return "", 0, errors.Wrapf(err, "Failed looking up image %q", image)
	}
	if len(dp) != 1 {
		return "", 0, errors.Errorf("bad descriptor tag %q", image)
	}
	blob, err := oci.FromDescriptor(context.Background(), dp[0].Descriptor())
	if err != nil {
		return "", 0, errors.Wrapf(err, "Error finding image")
	}
	defer blob.Close()
	desc := blob.Descriptor
	return desc.Digest.String(), desc.Size, nil
}

// PostManifest: Post an install.json.  Return the digest and size
// of the *manifest* describing the install.json blob, as that will
// be needed for the referring artifacts.
func PostManifest(path, dest string) (digest.Digest, int64, error) {
	r, err := NewDistRepo(dest)
	if err != nil {
		return "", 0,  errors.Wrapf(err, "Failed parsing destination address")
	}
	murl, err := r.findUrl(dest)
	if err != nil {
		return "", 0,  errors.Wrapf(err, "Failed parsing destination name")
	}

	// First, post an empty config and get the digest
	if err := murl.PostEmptyConfig(); err != nil {
		return "", 0,  err
	}

	// Post the actual install.json as a blob
	fSize, fDigest, err := murl.Post(path)
	if err != nil {
		return "", 0,  err
	}

	// Finally, build an ispec.Manifest
	config := ispec.Descriptor{
		MediaType: "application/vnd.unknown.config.v1+json",
		Digest: digest.NewDigestFromEncoded(digest.Canonical, "44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a"),
		Size: 2,
	}
	layers := []ispec.Descriptor{
		ispec.Descriptor{
			MediaType: "vnd.machine.install",
			Digest: fDigest,
			Size: fSize,
			Annotations: map[string]string{"org.opencontainers.image.title": "index.json"},
		},
	}
	t := time.Now().Format(time.RFC3339)
	m := ispec.Manifest{
		Versioned: specs.Versioned{ SchemaVersion: 2,},
		MediaType: ispec.MediaTypeImageManifest,
		Config: config,
		Layers: layers,
		Annotations: map[string]string{ispec.AnnotationCreated: t},
	}

	b, err := json.Marshal(&m)
	if err != nil {
		return "", 0,  errors.Wrapf(err, "Failed marshalling manifest")
	}
	mSize := int64(len(b))
	mDigest := digest.FromBytes(b)
	u := "http://" + murl.repo.addr + "/v2/" + murl.name + "/manifests/" + murl.tag
	req, err := http.NewRequest(http.MethodPut, u, bytes.NewBuffer(b))
	if err != nil {
		return "", 0,  errors.Wrapf(err, "Failed opening PUT request")
	}
	req.Header.Set("Content-Type", ispec.MediaTypeImageManifest)
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", 0,  errors.Wrapf(err, "Failed sending PUT request")
	}
	defer resp.Body.Close()

	resp.Body.Close()
	if resp.StatusCode != 201 {
		fmt.Printf("response code was %d, %q\n", resp.StatusCode, resp.Status)
		return "", 0,  errors.Wrapf(err, "Repo retunrred error for manifest wrapper.  Response was: %q", resp.Status)
	}
	return mDigest, mSize, nil
}

// PostArtifact: post the contents of filePath as an artifact referring to
// refDigest.  Since we've already run PostManifest, we know that
// the empty config (with digest emptyDigest) has certainly already been
// posted.
func PostArtifact(refDigest digest.Digest, refSize int64, path, mediatype, dest string) error {
	r, err := NewDistRepo(dest)
	if err != nil {
		return errors.Wrapf(err, "Failed parsing destination address")
	}
	murl, err := r.findUrl(dest)
	if err != nil {
		return errors.Wrapf(err, "Failed parsing destination name")
	}

	fSize, fDigest, err := murl.Post(path)
	if err != nil {
		return err
	}

	// Construct an ispec.Manifest referring to the blob
	config := ispec.Descriptor{
		MediaType: mediatype,
		Digest: digest.NewDigestFromEncoded(digest.Canonical, "44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a"),
		Size: 2,
	}
	layers := []ispec.Descriptor{
		ispec.Descriptor{
			MediaType: ispec.MediaTypeImageLayer,
			Digest: fDigest,
			Size: fSize,
			Annotations: map[string]string{"org.opencontainers.image.title": filepath.Base(path)},
		},
	}
	subject := ispec.Descriptor{
		MediaType: ispec.MediaTypeImageManifest,
		Digest: refDigest,
		Size: refSize,
	}
	t := time.Now().Format(time.RFC3339)
	manifest := ispec.Manifest{
		Versioned: specs.Versioned{ SchemaVersion: 2,},
		MediaType: ispec.MediaTypeImageManifest,
		Config: config,
		Layers: layers,
		Subject: &subject,
		Annotations: map[string]string{ispec.AnnotationCreated: t},
	}

	b, err := json.Marshal(&manifest)
	if err != nil {
		return errors.Wrapf(err, "Failed marshalling manifest")
	}
	bDigest := digest.FromBytes(b)
	u := "http://" + murl.repo.addr + "/v2/" + murl.name + "/manifests/" + bDigest.String()
	req, err := http.NewRequest(http.MethodPut, u, bytes.NewBuffer(b))
	if err != nil {
		return errors.Wrapf(err, "Failed opening PUT request")
	}
	req.Header.Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return errors.Wrapf(err, "Failed sending PUT request")
	}
	defer resp.Body.Close()

	resp.Body.Close()
	if resp.StatusCode != 201 {
		fmt.Printf("response code was %d, %q\n", resp.StatusCode, resp.Status)
		return errors.Wrapf(err, "Repo retunrred error for manifest wrapper.  Response was: %q", resp.Status)
	}
	return nil
}
