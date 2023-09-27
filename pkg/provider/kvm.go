package provider

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/apex/log"
	"github.com/pkg/errors"
	"github.com/project-machine/machine/pkg/api"
	"github.com/project-machine/machine/pkg/client"
	"github.com/project-machine/mos/pkg/trust"
)

// TODO - can we get machine to auto-detect the uefi-code it needs?
const KVMTemplate = `
    name: %s
    type: kvm
    ephemeral: false
    description: A fresh VM booting trust LiveCD in SecureBoot mode with TPM
    config:
      name: %s
      uefi: true
      uefi-code: /usr/share/OVMF/OVMF_CODE.secboot.fd
      uefi-vars: %s
      cdrom: %s
      boot: cdrom
      tpm: true
      gui: true
      serial: true
      tpm-version: 2.0
      secure-boot: true
      disks:
          - file: %s
            type: ssd
            size: 120G
          - file: %s
            format: raw
            type: hdd`

type KVMProvider struct {
}

type KVMMachine struct {
	Name    string
	Keyset  string // keyset to which this machine belongs
	Project string // key project to which this machine belongs
	UUID    string // assigned UUID (set in SUDI)
}

func NewKVMProvider() (KVMProvider, error) {
	if err := trust.RunCommand("machine", "list"); err != nil {
		return KVMProvider{}, errors.Wrapf(err, "machined not running?")
	}
	return KVMProvider{}, nil
}

func (p KVMProvider) Type() ProviderType {
	return KVMMachineType
}

func (p KVMProvider) Exists(mname string) bool {
	if err := trust.RunCommand("machine", "info", mname); err == nil {
		return true
	}
	return false
}

func (p KVMProvider) New(mname, keyproject, UUID string) (Machine, error) {
	s := strings.Split(keyproject, ":")
	if len(s) != 2 {
		return KVMMachine{}, errors.Errorf("Bad keyset project name %q", keyproject)
	}

	m := KVMMachine{
		Name:    mname,
		Keyset:  s[0],
		Project: s[1],
		UUID:    UUID,
	}

	machineBaseDir, err := trust.MachineDir(m.Name)
	if err != nil {
		return m, errors.Wrapf(err, "Failed getting machine dir")
	}
	if err := trust.EnsureDir(machineBaseDir); err != nil {
		return m, errors.Wrapf(err, "Failed getting machine dir")
	}

	// Create hard drive
	qcowPath := filepath.Join(machineBaseDir, fmt.Sprintf("%s.qcow2", m.Name))
	if err := trust.RunCommand("qemu-img", "create", "-f", "qcow2", qcowPath, "600G"); err != nil {
		return m, errors.Wrapf(err, "Failed creating disk")
	}

	keysetDir, projDir, err := trust.KeyProjectDir(m.Keyset, m.Project)
	if err != nil {
		return m, errors.Wrapf(err, "Failed finding keyset path")
	}

	sudiPath := filepath.Join(projDir, "sudi", UUID, "sudi.vfat")

	// Write a template
	// Note this is set to boot from provisioning ISO with sudi.vfat
	// attached.  We'll remove those after provisioning.  I did consider
	// creating without those, and setting those only if the user calls
	// Provision, but given the purpose of this machine type I'm not sure
	// that's worth it.
	provisionISO := filepath.Join(keysetDir, "artifacts", "provision.iso")
	uefiVars := filepath.Join(keysetDir, "bootkit", "ovmf-vars.fd")
	mData := fmt.Sprintf(KVMTemplate, m.Name, m.Name, uefiVars, provisionISO,
		qcowPath, sudiPath)
	_, _, err = trust.RunWithStdall(mData, "machine", "init", m.Name)
	if err != nil {
		return m, errors.Wrapf(err, "Failed initializing machine")
	}

	// Write out the details of the machine to persistent storage

	return m, nil
}

func (p KVMProvider) Delete(mname string) error {
	if err := trust.RunCommand("machine", "delete", mname); err != nil {
		return errors.Wrapf(err, "Failed deleting %q", mname)
	}
	return nil
}

func (m KVMMachine) waitForState(state string) error {
	for i := 1; i < 5; i += 1 {
		if m.state(state) {
			return nil
		}
		time.Sleep(time.Second * 1)
	}
	return errors.Errorf("Timed out waiting for %q to start", m.Name)
}

const (
	STOPPED string = "status: stopped"
	RUNNING string = "status: running"
	FAILED  string = "status: failed"
)

func (m KVMMachine) RunProvision() error {
	// Start the machine, and watch the console for 'provision complete'
	if err := m.Start(); err != nil {
		return errors.Wrapf(err, "Failed starting the machine to provision")
	}

	if err := m.waitForState(RUNNING); err != nil {
		return errors.Wrapf(err, "Error waiting for provisioning to begin")
	}

	//cmd := exec.Command("machine", "console", m.Name)
	home, err := os.UserHomeDir()
	if err != nil {
		return errors.Wrapf(err, "failed to get homedir")
	}
	mdir := filepath.Join(home, ".local/state/machine/machines", m.Name, m.Name)
	msock := filepath.Join(mdir, "sockets", "console.sock")
	time.Sleep(2 * time.Second)
	if err := waitForUnix(msock, "provisioned successfully", "XXX FAIL XXX"); err != nil {
		return errors.Wrapf(err, "Provisioning failed")
	}

	if err := m.waitForState(STOPPED); err != nil {
		return errors.Wrapf(err, "Machine did not shut down after provision")
	}

	return nil
}

func (m KVMMachine) updateForInstall() error {
	machine, rc, err := client.GetMachine(m.Name)
	if err != nil {
		return errors.Wrapf(err, "Failed to get machine")
	}

	if rc != 200 {
		return errors.Errorf("Error retrieving machine, error code %d", rc)
	}
	if !strings.HasSuffix(machine.Config.Cdrom, "provision.iso") {
		return errors.Errorf("Machine's cdrom was %q", machine.Config.Cdrom)
	}
	machine.Config.Cdrom = strings.TrimSuffix(machine.Config.Cdrom, "provision.iso")
	machine.Config.Cdrom = machine.Config.Cdrom + "install.iso"

	newDisks := []api.QemuDisk{}
	for _, d := range machine.Config.Disks {
		if strings.HasSuffix(d.File, "sudi.vfat") {
			_, projDir, err := trust.KeyProjectDir(m.Keyset, m.Project)
			if err != nil {
				return errors.Wrapf(err, "Failed finding keyset path")
			}
			d.File = filepath.Join(projDir, "sudi", m.UUID, "install.vfat")
		}
		newDisks = append(newDisks, d)
	}
	machine.Config.Disks = newDisks

	if err = client.PutMachine(machine); err != nil {
		return errors.Wrapf(err, "Failed to push updated machine")
	}
	return nil
}

func (m KVMMachine) updateForBoot() error {
	machine, rc, err := client.GetMachine(m.Name)
	if err != nil {
		return errors.Wrapf(err, "Failed to get machine")
	}

	if rc != 200 {
		return errors.Errorf("Error retrieving machine, error code %d", rc)
	}

	newDisks := []api.QemuDisk{}
	for _, d := range machine.Config.Disks {
		if strings.HasSuffix(d.File, "sudi.vfat") {
			continue
		}
		if strings.HasSuffix(d.File, "install.vfat") {
			continue
		}
		newDisks = append(newDisks, d)
	}
	machine.Config.Disks = newDisks

	machine.Config.Boot = "hdd"
	machine.Config.Cdrom = ""

	if err = client.PutMachine(machine); err != nil {
		return errors.Wrapf(err, "Failed to push updated machine")
	}
	return nil
}

func (m KVMMachine) RunInstall() error {
	log.Infof("Setting up to install %q\n", m.Name)
	if err := m.updateForInstall(); err != nil {
		return errors.Wrapf(err, "Failed updating %q for install", m.Name)
	}

	if err := m.Start(); err != nil {
		return errors.Wrapf(err, "Failed starting %q to install", m.Name)
	}

	if err := m.waitForState(RUNNING); err != nil {
		return errors.Wrapf(err, "Error waiting for install to begin")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return errors.Wrapf(err, "failed to get homedir")
	}
	mdir := filepath.Join(home, ".local/state/machine/machines", m.Name, m.Name)
	msock := filepath.Join(mdir, "sockets", "console.sock")
	time.Sleep(2 * time.Second)
	if err := waitForUnix(msock, "installed successfully", "XXX FAIL XXX"); err != nil {
		return errors.Wrapf(err, "Install failed")
	}

	if err := m.waitForState(STOPPED); err != nil {
		return errors.Wrapf(err, "Machine did not shut down after install")
	}

	if err := m.updateForBoot(); err != nil {
		return err
	}

	return nil
}

// Connect to unix socket @sockPath, and waith for either EOF,
// or for either @good or @string to be seen
func waitForUnix(sockPath, good, bad string) error {
	c, err := net.Dial("unix", sockPath)
	if err != nil {
		return errors.Wrapf(err, "Failed opening console socket %q", sockPath)
	}
	b, err := io.ReadAll(c)
	if err != nil {
		return errors.Wrapf(err, "Failed reading console socket")
	}
	s := string(b)
	if strings.Contains(s, good) {
		return nil
	}
	if strings.Contains(s, bad) {
		return errors.Errorf("Action failed, as %q was found", bad)
	}

	return errors.Errorf("Action timed out, did not find %q nor %q, in %q", good, bad)
}

func (m KVMMachine) state(desired string) bool {
	cmd := exec.Command("machine", "info", m.Name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}

	return strings.Contains(string(output), desired)
}

func (m KVMMachine) Start() error {
	if err := trust.RunCommand("machine", "start", m.Name); err != nil {
		return errors.Wrapf(err, "Failed starting %q", m.Name)
	}
	return nil
}

func (m KVMMachine) Stop() error {
	if err := trust.RunCommand("machine", "stop", m.Name); err != nil {
		return errors.Wrapf(err, "Failed stopping %q", m.Name)
	}
	return nil
}
