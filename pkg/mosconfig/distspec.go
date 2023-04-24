package mosconfig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	digest "github.com/opencontainers/go-digest"
	ispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
)

const (
	pubkeyArtifact = "vnd.machine.pubkeycrt"
	sigArtifact    = "vnd.machine.signature"
)

type DistUrl struct {
	name    string // the oci name (<name> in distribution spec)- e.g. foo/image
	tag     string // the oci tag, e.g. 1.0.0
	mDigest string // the digest for this image's manifest
	mSize   int64
	fDigest string // the digest for this file's contents - the actual blob digest
	repo    *DistRepo
}

type DistRepo struct {
	addr string // 10.0.2.2:5000 or /mnt/oci
}

// Pick out the name and tag from a url
func (r *DistRepo) findUrl(base string) (DistUrl, error) {
	url := DistUrl{}
	base = dropURLPrefix(base)
	s := strings.SplitN(base, "/", 2)
	if len(s) != 2 {
		return url, errors.Errorf("Failed parsing oci repo url: no '/'in %q", base)
	}
	s = strings.SplitN(s[1], ":", 2)
	if len(s) != 2 {
		return url, errors.Errorf("Failed parsing oci repo url: no ':'in %q", base)
	}
	url.name = s[0]
	url.tag = s[1]

	url.repo = r
	return url, nil
}

// Pick out the name and tag from a url, and get its size and digest.
// Obviously, this requires the url to exist.
func (r *DistRepo) openUrl(base string) (DistUrl, error) {
	url, err := r.findUrl(base)
	if err != nil {
		return url, err
	}

	// Get the image digests we need from,  e.g.
	// http://0.0.0.0:18080/v2/machine/install/manifests/1.0.0
	u := fmt.Sprintf("http://%s/v2/%s/manifests/%s", r.addr, url.name, url.tag)
	resp, err := http.Get(u)
	if err != nil {
		return url, errors.Wrapf(err, "Failed connecting to %q", u)
	}
	if resp.StatusCode != 200 {
		return url, errors.Errorf("Bad status code connecting to %q: %d", u, resp.StatusCode)
	}
	defer resp.Body.Close()

	// This is the digest we need to use to get the list of referrers
	url.mDigest = resp.Header.Get("Docker-Content-Digest")
	if url.mDigest == "" {
		return url, errors.Errorf("No Docker-Content-Digest received")
	}
	strSize := resp.Header.Get("Content-Length")
	url.mSize, err = strconv.ParseInt(strSize, 10, 64)

	// Read the actual ispec.Index and get the Digest for Layer 1 - that
	// is the actual digest of the blob we want
	manifest := ispec.Manifest{}
	err = json.NewDecoder(resp.Body).Decode(&manifest)
	if err != nil {
		return url, errors.Wrapf(err, "Failed parsing the install artifact manifest")
	}
	if len(manifest.Layers) == 0 {
		return url, errors.Errorf("No layers found in the install artifact manifest!")
	}
	if len(manifest.Layers) > 1 {
		return url, errors.Errorf("More than one layer found in the install artifact manifest.")
	}

	url.fDigest = manifest.Layers[0].Digest.String()

	return url, nil
}

func dropURLPrefix(url string) string {
	prefixes := []string{"docker://", "http://", "https://"}
	for _, p := range prefixes {
		if strings.HasPrefix(url, p) {
			url = url[len(p):]
		}
	}
	return url
}

// PingRepo checks whether a given dist url, say 10.0.2.2:5000, is
// up.
func PingRepo(base string) error {
	url := "http://" + base + "/v2/"
	resp, err := http.Get(url)
	if err != nil {
		return errors.Errorf("Failed connecting to %q", url)
	}
	if resp.StatusCode != 200 {
		return errors.Errorf("Bad status code %d connecting to %q: %d", url, resp.StatusCode)
	}
	resp.Body.Close()

	return nil
}

// Given a 10.0.2.2:5000/foo/install.json, set addr to
// http://10.0.2.2:5000, and check for connection using
// http://10.0.2.2:5000/v2
func NewDistRepo(base string) (*DistRepo, error) {
	r := DistRepo{}
	base = dropURLPrefix(base)
	s := strings.SplitN(base, "/", 2)
	if len(s) != 2 {
		return &r, errors.Errorf("Failed parsing oci repo url: no '/' in %q", base)
	}
	base = s[0]

	url := "http://" + base + "/v2/"
	resp, err := http.Get(url)
	if err != nil {
		return &r, errors.Errorf("Failed connecting to %q", url)
	}
	if resp.StatusCode != 200 {
		return &r, errors.Errorf("Bad status code %d connecting to %q: %d", url, resp.StatusCode)
	}
	defer resp.Body.Close()

	r.addr = base
	return &r, nil
}

func (r *DistRepo) FetchFile(path string, dest string) error {
	url := "http://" + r.addr + "/v2/" + path
	resp, err := http.Get(url)
	if err != nil {
		return errors.Errorf("Failed connecting to %q", url)
	}
	if resp.StatusCode != 200 {
		return errors.Errorf("Bad status code connecting to %q: %d", url, resp.StatusCode)
	}
	source := resp.Body
	defer source.Close()

	outf, err := os.OpenFile(dest, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer outf.Close()

	_, err = io.Copy(outf, source)
	if err != nil {
		return err
	}

	return nil
}

func (r *DistRepo) FetchInstall(disturl DistUrl, dest string) error {
	u := disturl.name + "/blobs/" + disturl.fDigest
	return r.FetchFile(u, dest)
}

// end-12b     GET     /v2/<name>/referrers/<digest>?artifactType=<artifactType>     200     404/400
func (r *DistRepo) GetReferrers(disturl DistUrl, artifactType string) (ispec.Index, error) {
	// Now fetch /v2/<name>/referrers/<digest>?artifactType=<artifactType>
	idx := ispec.Index{}
	u := fmt.Sprintf("http://%s/v2/%s/referrers/%s?artifactType=%s", r.addr, disturl.name, disturl.mDigest, artifactType)
	resp, err := http.Get(u)
	if err != nil {
		return idx, errors.Errorf("Failed connecting to %q", u)
	}
	if resp.StatusCode != 200 {
		return idx, errors.Errorf("Bad status code connecting to %q: %d", u, resp.StatusCode)
	}
	body := resp.Body
	defer body.Close()

	err = json.NewDecoder(body).Decode(&idx)
	if err != nil {
		return idx, errors.Wrapf(err, "Failed parsing the list of referrers")
	}

	if len(idx.Manifests) == 0 {
		return idx, errors.Errorf("No manifest for artifact type %v at %#v (queried url %q)", artifactType, disturl, u)
	}

	return idx, nil
}

func (r *DistRepo) fetchArtifact(disturl DistUrl, artifactType, dest string) error {
	referrer, err := r.GetReferrers(disturl, artifactType)
	if err != nil {
		return errors.Wrapf(err, "Failed getting list of referrers")
	}

	if len(referrer.Manifests) > 1 {
		// What do do?  Should we find one right here that the capath can verify?
		// Probably - but for now, just take the first one.
		fmt.Println("Warning: multiple referrers found, using first one")
	}

	digest := referrer.Manifests[0].Digest

	// we have the digest for a manifest whose layers[0] contains
	// the artifact we're looking for
	u := fmt.Sprintf("http://%s/v2/%s/blobs/%s", r.addr, disturl.name, digest)
	manifest := ispec.Manifest{}

	resp, err := http.Get(u)
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return errors.Errorf("bad response code from oci repo")
	}
	defer resp.Body.Close()
	err = json.NewDecoder(resp.Body).Decode(&manifest)
	if err != nil {
		return err
	}
	if len(manifest.Layers) == 0 {
		return errors.Errorf("Error parsing artifacts list")
	}

	digest = manifest.Layers[0].Digest
	u = fmt.Sprintf("%s/blobs/%s", disturl.name, digest)
	return r.FetchFile(u, dest)
}

func (r *DistRepo) FetchCert(disturl DistUrl, dest string) error {
	return r.fetchArtifact(disturl, pubkeyArtifact, dest)
}

func (r *DistRepo) FetchSignature(disturl DistUrl, dest string) error {
	return r.fetchArtifact(disturl, sigArtifact, dest)
}

func getSizeDigestDist(inUrl string) (string, int64, error) {
	// http://127.0.0.1:18080/v2/os/busybox-squashfs/manifests/1.0
	r, err := NewDistRepo(inUrl)
	if err != nil {
		return "", 0, errors.Wrapf(err, "Failed to find source repo info for %q", inUrl)
	}

	u, err := r.openUrl(inUrl)
	if err != nil {
		return "", 0, errors.Wrapf(err, "Error parsing install manifest inUrl %q", inUrl)
	}

	return u.mDigest, u.mSize, nil
}

func getSizeDigest(inUrl string) (string, int64, error) {
	if strings.HasPrefix(inUrl, "oci:") {
		return getSizeDigestOCI(inUrl)
	}
	return getSizeDigestDist(inUrl)
}

// An empty digest, consisting of "{}", is required for an artifact.
// serge@jerom ~$ echo -n '{}' | sha256sum
// 44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a  -
const emptyDigest = "sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a"

func (disturl *DistUrl) PostEmptyConfig() error {
	url := "http://" + disturl.repo.addr + "/v2/" + disturl.name + "/blobs/uploads/?digest=" + emptyDigest
	buf := bytes.NewBufferString("{}")
	resp, err := http.Post(url, "application/octet-stream", buf)
	if err != nil {
		return errors.Wrapf(err, "Failed posting empty config")
	}
	resp.Body.Close()
	if resp.StatusCode != 201 {
		return errors.Wrapf(err, "Failed posting empty config. Response was: %q", resp.Status)
	}
	return nil
}

func (disturl *DistUrl) Post(path string) (int64, digest.Digest, error) {
	d := digest.FromString("")
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, d, errors.Wrapf(err, "Failed reading %q", path)
	}
	fSize := int64(len(b))
	fDigest := digest.FromBytes(b)

	u := "http://" + disturl.repo.addr + "/v2/" + disturl.name + "/blobs/uploads/?digest=" + fDigest.String()
	buf := bytes.NewBuffer(b)
	resp, err := http.Post(u, "application/octet-stream", buf)
	if err != nil {
		return fSize, fDigest, errors.Wrapf(err, "Failed writing manifest as blob")
	}
	resp.Body.Close()
	if resp.StatusCode != 201 {
		return fSize, fDigest, errors.Wrapf(err, "Repo returned error for manifest as blob.  Response was: %q", resp.Status)
	}

	return fSize, fDigest, nil
}

// remoteManifest: fetch an install.json manifest from an
// OCI distribution spec remote.  Get the signature and
// certificate from referring artifacts, verify the signature,
// and verify that the certificate is signed by our trusted CA.
//
// TODO - right now we only allow this if the host is not yet
// installed.  It's debatable whether we want to support it after
// install as well, for quick activation of a remote target.  But
// I think requiring a proper install makes sense at that point.
// Which means this is only used for livecds.
func (mos *Mos) remoteManifest(url string) (*InstallFile, error) {
	if PathExists(filepath.Join(mos.opts.ConfigDir, "manifest.git")) {
		return &InstallFile{}, errors.Errorf("Opening remote manifest on installed host is unsupported")
	}

	is := InstallSource{}
	if err := is.FetchFromZot(url); err != nil {
		return &InstallFile{}, errors.Wrapf(err, "Error fetching remote manifest")
	}
	defer is.Cleanup()

	manifest, err := ReadVerifyInstallManifest(is, mos.opts.CaPath, mos.storage)
	if err != nil {
		return &InstallFile{}, errors.Wrapf(err, "Error verifying remote manifest")
	}

	return &manifest, nil
}

func dropHashPrefix(d string) string {
	d = strings.TrimPrefix(d, "sha256:")
	d = strings.TrimPrefix(d, "sha512:")
	return d
}
