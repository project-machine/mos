package mosconfig

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	ispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/opencontainers/umoci"
	"github.com/opencontainers/umoci/mutate"
	"github.com/opencontainers/umoci/oci/casext"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"

	"github.com/project-machine/trust/pkg/trust"

	satomfs "stackerbuild.io/stacker/pkg/atomfs"
	stackeroci "stackerbuild.io/stacker/pkg/oci"
	"stackerbuild.io/stacker/pkg/squashfs"
	imagesource "stackerbuild.io/stacker/pkg/types"
)

func MountOCILayer(ocidir, name, dest string) (func(), error) {
	metadir := filepath.Join(dest, "meta") // directory for atomfs metadata
	if err := EnsureDir(metadir); err != nil {
		return func() {}, errors.Errorf("Failed creating metadata directory")
	}

	cleanup := func() { os.RemoveAll(metadir) }

	// If $ocidir/index.json exists, then it's a simple oci layout.
	// If not, then it's a local zot layout.
	// For image atomix/baseos:1.0.0, if it's OCI layout then the
	// image will be called "atomix/baseos:1.0.0" in $ocidir/index.json.
	// Otherwise, it will be called "1.0.0" in
	// $ocidir/atomix/baseos/index.json.
	if !PathExists(filepath.Join(ocidir, "index.json")) {
		idx := strings.LastIndex(name, ":")
		if idx == -1 {
			return func() {}, fmt.Errorf("A version must be specified for zot layout")
		}
		ocidir = filepath.Join(ocidir, name[:idx])
		name = name[idx+1:]
	}
	opts := satomfs.MountOCIOpts{
		OCIDir:       ocidir,
		MetadataPath: metadir,
		Tag:          name,
		Target:       dest,
	}
	mol, err := satomfs.BuildMoleculeFromOCI(opts)
	if err != nil {
		return cleanup, err
	}

	err = mol.Mount(dest)
	if err != nil {
		return cleanup, errors.Wrapf(err, "Failed mounting atomfs")
	}
	cleanup = func() {
		satomfs.Umount(dest)
		os.RemoveAll(metadir)
	}

	return cleanup, nil
}

// MountRepoLayer mounts an image path @name at directory @dest.
// @repobase is the repository base to prepend to @name to find the
// layer.  If it starts with 'oci:', then it is a simple oci layout
// base.  If it starts with 'zot:', then it is the top level zot
// directory (under which the oci images will be in oci layouts at
// subdirectories mirroring the image name, for instance ubuntu/amd64.
// For 'docker:', it will be a directory to which we will sync the
// remote images in zot layout.
func MountRepoLayer(repobase, name, dest string) (string, func(), error) {
	s := strings.SplitN(repobase, ":", 2)
	if len(s) != 2 {
		return "", func() {}, fmt.Errorf("Repo-base type not specified")
	}
	repotype := s[0]
	rest := s[1]

	switch repotype {
	case "docker":
		return "", func() {}, fmt.Errorf("Docker repo bases not yet handed")
	case "oci":
		cachedir := rest
		cleanup, err := MountOCILayer(rest, name, dest)
		return cachedir, cleanup, err
	default:
		return "", func() {}, fmt.Errorf("Unknown repo base type: %q", repotype)
	}
}

func MountSOCI(repobase, metalayer, capath, mountpoint string) error {
	tmpd, err := os.MkdirTemp("", "extract")
	if err != nil {
		return errors.Wrapf(err, "Failed creating tempdir")
	}

	fmt.Printf("XXX - mounting meta layer %q %q at %q\n", repobase, metalayer, tmpd)
	storagecache, cleanup, err := MountRepoLayer(repobase, metalayer, tmpd)
	if err != nil {
		return errors.Wrapf(err, "Failed unpacking SOCI metalayer layer")
	}
	defer cleanup()

	fmt.Printf("XXX - successfully mounted the repo layer\n")

	mPath := filepath.Join(tmpd, "manifest.yaml")
	sPath := mPath + ".signed"
	cPath := filepath.Join(tmpd, "manifestCert.pem")
	if !PathExists(mPath) || !PathExists(sPath) || !PathExists(cPath) {
		return errors.Errorf("bad SOCI layer")
	}

	// Set up a temporary storage
	opts := DefaultMosOptions()
	opts.CaPath = capath

	opts.StorageCache = storagecache

	s, err := NewStorage(opts)
	if err != nil {
		return err
	}
	manifest, err := ReadVerifyManifest(mPath, cPath, capath, "", s)
	if err != nil {
		fmt.Printf("Failed verifying %q using %q and %q\n", mPath, cPath, capath)
		return errors.Wrapf(err, "Verification of manifest on metalayer failed")
	}

	if len(manifest.Targets) != 1 {
		return errors.Errorf("manifest.yaml must have precisely one target")
	}
	t := manifest.Targets[0]

	name := t.ImagePath
	if t.Version != "" {
		name = name + ":" + t.Version
	}
	_, _, err = MountRepoLayer(repobase, name, mountpoint)
	if err != nil {
		return errors.Wrapf(err, "Failed mounting %s (%s %s)", t.ServiceName, repobase, name)
	}

	return nil
}

// SOCI, or "signed oci", is an OCI layer which contains:
//   - mos install manifest
//   - manifest signature
//   - public key cert for verifying the manifest
type SOCI struct {
	// Layer is the OCI layer referenced in the manifest
	Layer string

	// Name by which the service will be known, probably the
	// base image name of Layer
	ServiceName string

	// Path under which the layer is found on the host
	ImagePath string

	// Version to call the service.
	Version string

	// Meta is the name we are giving the signed oci layer,
	// which is itself an OCI layer
	Meta string

	// The certificate for verifying the signed  manifest
	Cert string

	// The key for signing the manifest.  This is only required
	// when creating, of course
	Key string
}

// Open an OCI image manifest.  Return the ispec.Manifest descriptor as
// well as the shasum.
func openManifest(url string) (ispec.Manifest, string, error) {
	emptyM := ispec.Manifest{}
	src, err := imagesource.NewImageSource(url)
	if err != nil {
		return emptyM, "", err
	}
	if src.Type == imagesource.DockerLayer {
		return emptyM, "", fmt.Errorf("docker:// urls not yet supported")
	}

	r := strings.SplitN(src.Url, ":", 2)
	if len(r) != 2 {
		return emptyM, "", fmt.Errorf("Invalid image url")
	}
	ociDir := r[0]
	imageName := r[1]
	oci, err := umoci.OpenLayout(ociDir)
	if err != nil {
		return emptyM, "", err
	}
	defer oci.Close()

	ociManifest, err := stackeroci.LookupManifest(oci, imageName)
	if err != nil {
		return emptyM, "", err
	}

	descriptorPaths, err := oci.ResolveReference(context.Background(), imageName)
	if err != nil {
		return emptyM, "", err
	}

	if len(descriptorPaths) != 1 {
		return emptyM, "", fmt.Errorf("bad descriptor %q in %q", imageName, ociDir)
	}

	blob, err := oci.FromDescriptor(context.Background(), descriptorPaths[0].Descriptor())
	if err != nil {
		return emptyM, "", err
	}
	defer blob.Close()

	if blob.Descriptor.MediaType != ispec.MediaTypeImageManifest {
		return emptyM, "", fmt.Errorf("descriptor does not point to a manifest: %s", blob.Descriptor.MediaType)
	}

	shasum := blob.Descriptor.Digest.Encoded()

	return ociManifest, shasum, nil
}

// Make an SOCI layer
// layer: the oci or zot url to the OCI layer to "wrap"
func (soci *SOCI) Generate() error {
	tmpdir, err := os.MkdirTemp("", "soci")
	if err != nil {
		return fmt.Errorf("Failed generating temporary working directory: %w", err)
	}
	defer os.RemoveAll(tmpdir)

	if err := EnsureDir(filepath.Join(tmpdir, "oci")); err != nil {
		return fmt.Errorf("Error creating oci directory: %w", err)
	}

	switch {
	case strings.HasPrefix(soci.Layer, "oci:"):
		break
	case strings.HasPrefix(soci.Layer, "docker:"):
		return fmt.Errorf("FIXME: remote images are not yet supported")
	default:
		return fmt.Errorf("Unknown image url: %q", soci.Layer)
	}

	_, shasum, err := openManifest(soci.Layer)
	if err != nil {
		return fmt.Errorf("Failed opening oci layer %q: %w", soci.Layer, err)
	}

	t := Target{
		ServiceName:  soci.ServiceName,
		ImagePath:    soci.ImagePath,
		Version:      soci.Version,
		ServiceType:  HostfsService,
		NSGroup:      "",
		Network:      TargetNetwork{HostNetwork},
		ManifestHash: shasum,
	}
	fmt.Printf("XXX shasum is %s\n", shasum)

	target := InstallTargets{t}
	manifest := InstallFile{
		Version:     1,
		ImageType:   ISO,
		UpdateType:  FullUpdate,
		Product:     "de6c82c5-2e01-4c92-949b-a6545d30fc06", // FIXME get this from cert?
		Targets:     target,
		StorageType: AtomfsStorageType,
	}

	// write the manifest
	bytes, err := yaml.Marshal(&manifest)
	if err != nil {
		return fmt.Errorf("Error marshaling manifest: %w", err)
	}
	mPath := filepath.Join(tmpdir, "manifest.yaml")
	if err := os.WriteFile(mPath, bytes, 0644); err != nil {
		return fmt.Errorf("Error writing manifest to %q: %w", mPath, err)
	}

	// copy the cert
	if err := CopyFileBits(soci.Cert, filepath.Join(tmpdir, "manifestCert.pem")); err != nil {
		return fmt.Errorf("Error copying manifest signing cert: %w", err)
	}

	// write the signed manifest
	sPath := mPath + ".signed"
	if err = trust.Sign(mPath, sPath, soci.Key); err != nil {
		return fmt.Errorf("Error signing manifest: %w", err)
	}

	if err := createLayer(soci.Meta, tmpdir); err != nil {
		return fmt.Errorf("Error creating final meta oci layer: %w", err)
	}
	return nil
}

func splitOCIURL(url string) (string, string, error) {
	src, err := imagesource.NewImageSource(url)
	if err != nil {
		return "", "", err
	}
	if src.Type == imagesource.DockerLayer {
		return "", "", fmt.Errorf("docker:// urls not yet supported")
	}

	r := strings.SplitN(src.Url, ":", 2)
	if len(r) != 2 {
		return "", "", fmt.Errorf("Invalid image url")
	}
	return r[0], r[1], nil
}

func doInsert(oci casext.Engine, toInsert string, insertAt string, tag string) error {
	dps, err := oci.ResolveReference(context.Background(), tag)
	if err != nil {
		return err
	}

	mutator, err := mutate.New(oci, dps[0])
	if err != nil {
		return err
	}

	now := time.Now()
	history := &ispec.History{
		CreatedBy: "project-machine OS creator",
		Created:   &now,
	}

	// we can't use mutator here because it compresses
	// stuff, so we do this in a roundabout way.
	tmpd, err := os.MkdirTemp("", "mos-builder-insert-squashfs-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpd)

	// We make the top level dir with 0755 rather than having
	// the squashfs's top level dir inherit the TempDir perms (700).
	rootfs := filepath.Join(tmpd, "root")
	if err := os.Mkdir(rootfs, 0755); err != nil {
		return err
	}

	err = filepath.Walk(toInsert, func(curPath string, info os.FileInfo, err error) error {
		// This really needs a better copyTree like function.  For now symlinks will
		// not be honored and probably other issues.
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		pathInSquashfs := filepath.Join(rootfs, insertAt, curPath[len(toInsert):])
		return CopyFileBits(curPath, pathInSquashfs)
	})
	if err != nil {
		return errors.Wrapf(err, "error copying file into squashfs buffer dir")
	}

	blob, mediaType, rootHash, err := squashfs.MakeSquashfs("", rootfs, nil, squashfs.VerityMetadataPresent)
	if err != nil {
		return err
	}
	defer blob.Close()

	annotations := map[string]string{}
	annotations[squashfs.VerityRootHashAnnotation] = rootHash

	_, err = mutator.Add(context.Background(), mediaType, blob, history, mutate.NoopCompressor, annotations)
	if err != nil {
		return err
	}

	ndp, err := mutator.Commit(context.Background())
	if err != nil {
		return err
	}

	err = oci.UpdateReference(context.Background(), tag, ndp.Root())
	if err != nil {
		return err
	}

	return nil
}

// Create an OCI layer from a directory's contents
// TODO - if the Meta layer is not a local oci layout, then should write to
// a tempdir and then copy over to final destination.  For now, we just
// require it be oci.
func createLayer(destUrl string, sourceDir string) error {
	ocidir, ociname, err := splitOCIURL(destUrl)
	if err != nil {
		return fmt.Errorf("Failure parsing url: %w", err)
	}

	var oci casext.Engine
	if PathExists(filepath.Join(ocidir, "index.json")) {
		oci, err = umoci.OpenLayout(ocidir)
	} else {
		oci, err = umoci.CreateLayout(ocidir)
	}
	if err != nil {
		return fmt.Errorf("Error opening oci layout at %q: %w", ocidir, err)
	}
	defer oci.Close()

	if err := umoci.NewImage(oci, ociname); err != nil {
		return fmt.Errorf("Error creating new image %q under %q: %w", ociname, ocidir, err)
	}

	if err := doInsert(oci, sourceDir, "/", ociname); err != nil {
		return fmt.Errorf("Error inserting directory %q into %q:%q: %w", sourceDir, ocidir, ociname, err)
	}

	return nil
}
