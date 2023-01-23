package mosconfig

// this contains our init related code
// purely systemd for now

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/apex/log"
)

func systemdStart(unitName string) error {
	if err := RunCommand("systemctl", "enable", unitName); err != nil {
		return fmt.Errorf("Failed enabling %s: %w", unitName, err)
	}
	if err := RunCommand("systemctl", "start", "--no-block", unitName); err != nil {
		return fmt.Errorf("Failed starting %s: %w", unitName, err)
	}
	return nil
}

const execServiceTemplate = `
[Unit]
Description=%s
DefaultDependencies=no
After=network-online.target cloud-init.target
Wants=network.target

[Service]
Restart=on-failure
RestartSec=1
ExecStart=/usr/bin/lxc-execute -n %s
ExecStop=/usr/bin/lxc-stop -n %s

[Install]
WantedBy=multi-user.target
`

const stopService = `
[Unit]
Description=Stop the %s container before shutdown
DefaultDependencies=no
Before=shutdown.target

[Service]
Type=oneshot
ExecStart=/usr/bin/systemctl stop %s
TimeoutStartSec=0

[Install]
WantedBy=shutdown.target
`

func (mos *Mos) writeContainerService(t *Target) error {
	unitName := fmt.Sprintf("%s.service", t.ServiceName)
	dest := filepath.Join(mos.opts.RootDir, "/etc", "systemd", "system", unitName)
	log.Infof("Writing container service at %q", dest)
	os.Remove(dest)
	content := []byte(fmt.Sprintf(execServiceTemplate, t.ServiceName, t.ServiceName, t.ServiceName))
	if err := os.WriteFile(dest, content, 0644); err != nil {
		return fmt.Errorf("Failed writing systemd.service file for %q: %w", unitName, err)
	}

	return nil
}

func (mos *Mos) startInit(t *Target) error {
	unitName := fmt.Sprintf("%s.service", t.ServiceName)
	if err := systemdStart(unitName); err != nil {
		return err
	}
	return nil
}
