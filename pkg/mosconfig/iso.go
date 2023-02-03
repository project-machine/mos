package mosconfig

import (
	"context"
        "fmt"
        "os"
	"path/filepath"

	ispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/opencontainers/umoci"
	"github.com/project-machine/trust/pkg/trust"
	"gopkg.in/yaml.v2"
	"stackerbuild.io/stacker/pkg/lib"
)

type ISOConfig struct {
	InputFile  string
	OutputFile string
	Cert       string
	Key        string
	UpdateType UpdateType
	Product    string
}

func (iso *ISOConfig) Generate() error {
        if !PathExists(iso.InputFile) {
                return fmt.Errorf("Target file %q not found", iso.InputFile)
        }
        if !PathExists(iso.Cert) {
                return fmt.Errorf("Manifest signing certificate not found")
        }
        if !PathExists(iso.Key) {
                return fmt.Errorf("Manifest signing key not found")
        }
        if PathExists(iso.OutputFile) {
                return fmt.Errorf("Output file %q exists, not overwriting", iso.OutputFile)
        }

        dir, err := os.MkdirTemp("", "mos-iso")
        if err != nil {
                return err
        }
        defer os.RemoveAll(dir)

        err = CopyFileBits(iso.Cert, filepath.Join(dir, "install.pem"))
        if err != nil {
                return fmt.Errorf("Failure copying certificate into ISO")
        }

        manifest, inputTargets, err := ManifestFromTargets(iso.InputFile)
        if err != nil {
                return fmt.Errorf("Failure creating manifest from target list: %w", err)
        }

        // Since we're making an old-school iso image, we'll use an
        // old-school oci layout
        ociDir := filepath.Join(dir, "oci")
        if err = EnsureDir(ociDir); err != nil {
                return err
        }

        for key, t := range inputTargets {
                sum, err := copyToOcidir(t.ImagePath, t.ServiceName, ociDir)
		if err != nil {
                        return err
                }
		manifest.Targets[key].ManifestHash = sum
        }

	manifest.Version = CurrentInstallFileVersion
	manifest.ImageType = ISO
	manifest.Product = iso.Product
	manifest.StorageType = AtomfsStorageType
	manifest.UpdateType = iso.UpdateType

        bytes, err := yaml.Marshal(&manifest)
        if err != nil {
                return fmt.Errorf("Failure serializing the install manifest")
        }

        mPath := filepath.Join(dir, "install.yaml")
        if err = os.WriteFile(mPath, bytes, 0640); err != nil {
                return fmt.Errorf("Failed writing out install.yaml: %w", err)
        }

        sPath := filepath.Join(dir, "install.yaml.signed")
        if err = trust.Sign(mPath, sPath, iso.Key); err != nil {
                return fmt.Errorf("Failed signing the install manifest: %w", err)
        }

        // TODO - create an efi directory and copy the UKI and shim

        // Create the ISO
        cmd := []string{
                "xorriso",
                "-compliance", "iso_9660_level=3",
                "-as", "mkisofs",
                "-V", "MOS-INSTALL",
                // "-e", "loader/images/efi-esp.img", //Not doing this yet...
                "-isohybrid-gpt-basdat",
                "-partition_cyl_align", "all",
                "-no-emul-boot", "-isohybrid-gpt-basdat",
                "-o", iso.OutputFile,
                dir}
        if err = RunCommand(cmd...); err != nil {
                return fmt.Errorf("Error creating ISO.\nCommand: %#v\nError: %w", cmd, err)
        }

        return nil
}

// copyToOcidir - copy a layer from the oci image path or docker url
// specified in the install yaml, into the ISO/oci/ directory.
// Also get the shasum of the image manifest, and return that as the
// first argument.
func copyToOcidir(src, name, ociDir string) (string, error) {
	dest := fmt.Sprintf("oci:%s:%s", ociDir, name)
	copyOpts := lib.ImageCopyOpts{Src: src, Dest: dest, Progress: os.Stdout}
	if err := lib.ImageCopy(copyOpts); err != nil {
		return "", fmt.Errorf("failed copying layer %q to %q: %w", src, dest, err)
	}

	oci, err := umoci.OpenLayout(ociDir)
	if err != nil {
		return "", err
	}
	defer oci.Close()

	descriptorPaths, err := oci.ResolveReference(context.Background(), name)
	if err != nil {
		return "", err
	}

	if len(descriptorPaths) != 1 {
		return "", fmt.Errorf("bad descriptor %q in %q", name, ociDir)
	}

	blob, err := oci.FromDescriptor(context.Background(), descriptorPaths[0].Descriptor())
	if err != nil {
		return"", err
	}
	defer blob.Close()

	if blob.Descriptor.MediaType != ispec.MediaTypeImageManifest {
		return "", fmt.Errorf("descriptor does not point to a manifest: %s", blob.Descriptor.MediaType)
	}

	shasum := blob.Descriptor.Digest.Encoded()

	return shasum, nil
}
