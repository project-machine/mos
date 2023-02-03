package mosconfig

import (
	"fmt"
	"path/filepath"
	"strings"
)

func InitializeMos(storeDir, configDir, configFile string) error {
	// We must have $basedir/install.yml and $basedir/cert.pem
	baseDir := filepath.Dir(configFile)
	cPath := filepath.Join(baseDir, "manifestCert.pem")
	sPath := filepath.Join(baseDir, "install.yaml.signed")
	caPath := filepath.Join(baseDir, "manifestCA.pem")
	if !PathExists(configFile) || !PathExists(cPath) || !PathExists(sPath) || !PathExists(caPath) {
		return fmt.Errorf("Install manifest or certificate missing")
	}

	mos, err := NewMos(configDir, storeDir)
	if err != nil {
		return fmt.Errorf("Error opening manifest: %w", err)
	}
	defer mos.Close()

	// Well, bit of a chicken and egg problem here.  We parse the configfile
	// first so we can copy all the needed zot images.
	cf, err := simpleParseInstall(configFile)
	if err != nil {
		return fmt.Errorf("Failed parsing install configuration")
	}

	for _, target := range cf.Targets {
		err = mos.storage.ImportTarget(baseDir, &target)
		if err != nil {
			return err
		}
	}

	if cf.UpdateType == PartialUpdate {
		return fmt.Errorf("Cannot install with a partial manifest")
	}

	// Finally set up our manifest store
	// The manifest will be re-read as it is verified.
	err = mos.initManifest(configFile, cPath, caPath, configDir)
	if err != nil {
		return fmt.Errorf("Error initializing system manifest: %w", err)
	}

	return nil
}

// return the fullname and version from a zot url.  For instance,
// fullnameFromUrl("docker://zothub.io/c3/base:latest") returns
// "c3/base", "latest", nil
func fullnameFromUrl(url string) (string, string, error) {
	prefix := "docker://"
	prefixLen := len(prefix)
	if !strings.HasPrefix(url, prefix) {
		return "", "", fmt.Errorf("Bad image URL: bad prefix")
	}
	url = url[prefixLen:]
	addrsplit := strings.SplitN(url, "/", 2)
	if len(addrsplit) < 2 {
		return "", "", fmt.Errorf("Bad image URL: no address")
	}
	tagname := addrsplit[1]
	idx := strings.LastIndex(tagname, ":")
	if idx == -1 {
		return "", "", fmt.Errorf("Bad image URL: no tag")
	}
	name := tagname[:idx]
	version := tagname[idx+1:]
	if len(name) < 1 || len(version) < 1 {
		return "", "", fmt.Errorf("Bad image URL: short name or tag")
	}
	return name, version, nil
}
