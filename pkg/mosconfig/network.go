package mosconfig

import (
	"fmt"
	"strings"

	"github.com/apex/log"
	"github.com/pkg/errors"
	"github.com/project-machine/mos/pkg/utils"
)

// Validate a network config during manifest load.  At this point we
// cannot yet verify whether resources like ports and addresses are in
// use, as that may change before service start.
func (t Target) ValidateNetwork() bool {
	n := t.Network
	switch n.Type {
	case HostNetwork, NoNetwork:
		return true
	case SimpleNetwork:
		break
	default:
		return false
	}

	for _, p := range n.Ports {
		if p.HostPort > 65536 || p.ContainerPort > 65536 {
			log.Warnf("too-high port %d or %d", p.HostPort, p.ContainerPort)
			return false
		}
		if p.HostPort == 0 || p.ContainerPort == 0 {
			log.Warnf("zero port %d or %d", p.HostPort, p.ContainerPort)
			return false
		}
	}

	return true
}

type SimplePort struct {
	HostPort      uint `json:"host" yaml:"host"`
	ContainerPort uint `json:"container" yaml:"container"`
}

// Pick a lxcbr0 address which is not yet in use.
// For now assume lxcbr0 is 10.0.3.1, TODO we should autodetect this.
func (mos *Mos) UnusedAddress() (string, error) {
	for c := 3; c < 255; c++ {
		txt := fmt.Sprintf("10.0.3.%d", c)
		if _, ok := mos.Manifest.IpAddrs[txt]; !ok {
			return txt, nil
		}
	}
	return "", fmt.Errorf("No available addresses")
}

func (mos *Mos) setupSimpleNet(t *Target) ([]string, error) {
	config := []string{"lxc.net.0.type = veth",
		"lxc.net.0.type = veth",
		"lxc.net.0.link = lxcbr0",
		"lxc.net.0.flags = up",
		"lxc.net.0.hwaddr = 00:16:3e:xx:xx:xx",
	}

	mos.NetLock.Lock()
	defer mos.NetLock.Unlock()

	ipv4 := t.Network.Address
	ipv6 := t.Network.Address6

	// Make sure any requested address is not in use
	if ipv4 != "" {
		if _, ok := mos.Manifest.IpAddrs[ipv4]; ok {
			return config, fmt.Errorf("Address in use: %q", ipv4)
		}
	}
	if ipv6 != "" {
		if _, ok := mos.Manifest.IpAddrs[ipv6]; ok {
			return config, fmt.Errorf("Address in use: %q", ipv6)
		}
	}

	// If no address requested, choose one.  No dhcp, because port fwd...
	if ipv4 == "" && ipv6 == "" {
		var err error
		ipv4, err = mos.UnusedAddress()
		if err != nil {
			return config, err
		}
	}

	if ipv4 != "" {
		config = append(config, "lxc.net.0.ipv4.address = "+ipv4)
		config = append(config, "lxc.environment = IPV4="+ipv4)
		mos.Manifest.IpAddrs[ipv4] = t.ServiceName
	}

	if ipv6 != "" {
		config = append(config, "lxc.net.0.ipv6.address = "+ipv6)
		config = append(config, "lxc.environment = IPV6="+ipv6)
		mos.Manifest.IpAddrs[ipv6] = t.ServiceName
	}

	if err := mos.setupPortFwd(t); err != nil {
		if ipv4 != "" {
			delete(mos.Manifest.IpAddrs, ipv4)
		}
		if ipv6 != "" {
			delete(mos.Manifest.IpAddrs, ipv6)
		}
		return config, err
	}

	return config, nil
}

// must be called with mos.NetLock held, since we update mos.Manifest
func (mos *Mos) DefaultNic() (string, error) {
	if mos.Manifest.DefaultNic != "" {
		return mos.Manifest.DefaultNic, nil
	}
	out, err := utils.Run("ip", "route")
	if err != nil {
		return "", errors.Wrapf(err, "Failed getting default route")
	}

	lines := strings.Split(string(out), "\n")
	for _, l := range lines {
		if !strings.HasPrefix(l, "default via") {
			continue
		}
		s := strings.Split(l, " ")
		if len(s) < 5 {
			continue
		}
		if s[3] != "dev" {
			continue
		}
		nic := s[4]
		mos.Manifest.DefaultNic = nic
		return nic, nil
	}

	return "", fmt.Errorf("No default route found (%q)", out)
}

// Setup port forward rules for a container.  Must be called with the
// mos.NetLock held.  mos.setupSimpleNet() takes that lock.
func (mos *Mos) setupPortFwd(t *Target) error {
	nic, err := mos.DefaultNic()
	if err != nil {
		return errors.Wrapf(err, "Failed to find default nic")
	}
	ipaddr, err := t.Ipaddr()
	if err != nil {
		return err
	}
	for _, p := range t.Network.Ports {
		destaddr := strings.Split(ipaddr, "/")[0] // 192.168.2.0/24
		destaddr = fmt.Sprintf("%s:%d", destaddr, p.ContainerPort)
		cmd := []string{
			"iptables", "-t", "nat", "-A", "PREROUTING", "-p", "tcp",
			"-i", nic, "--dport", fmt.Sprintf("%d", p.HostPort),
			"-j", "DNAT", "--to-destination", destaddr,
			"-m", "comment", "--comment", t.ServiceName}
		if err := utils.RunCommand(cmd...); err != nil {
			return errors.Wrapf(err, "Failed setting up port forward for %#v", p)
		}
	}
	return nil
}

func (mos *Mos) SetupTargetNetwork(t *Target) ([]string, error) {
	switch t.Network.Type {
	case HostNetwork:
		return []string{"lxc.net.0.type = none"}, nil
	case NoNetwork:
		return []string{"lxc.net.0.type = empty"}, nil
	case SimpleNetwork:
		return mos.setupSimpleNet(t)
	default:
		return []string{}, fmt.Errorf("Unhandled network type: %s", t.Network.Type)
	}
}

func (t *Target) Ipaddr() (string, error) {
	if t.Network.Address != "" {
		return t.Network.Address, nil
	}
	if t.Network.Address6 != "" {
		return "[" + t.Network.Address6 + "]", nil
	}

	return "", fmt.Errorf("No usable address for port forward destination")
}

func (mos *Mos) StopTargetNetwork(t *Target) error {
	mos.NetLock.Lock()
	defer mos.NetLock.Unlock()

	ipaddr := ""
	nic := ""
	for _, p := range t.Network.Ports {
		if ipaddr == "" {
			var err error
			ipaddr, err = t.Ipaddr()
			if err != nil {
				return err
			}
			nic, err = mos.DefaultNic()
			if err != nil {
				return errors.Wrapf(err, "Failed to find default nic")
			}
		}

		destaddr := strings.Split(ipaddr, "/")[0] // 192.168.2.0/24
		destaddr = fmt.Sprintf("%s:%d", destaddr, p.ContainerPort)

		cmd := []string{
			"iptables", "-t", "nat", "-D", "PREROUTING", "-p", "tcp",
			"-i", nic, "--dport", fmt.Sprintf("%d", p.HostPort),
			"-j", "DNAT", "--to-destination", destaddr,
			"-m", "comment", "--comment", t.ServiceName}
		if err := utils.RunCommand(cmd...); err != nil {
			return errors.Wrapf(err, "Failed setting up port forward for %#v", p)
		}
		delete(mos.Manifest.UsedPorts, p.HostPort)
	}

	if t.Network.Address != "" {
		delete(mos.Manifest.IpAddrs, t.Network.Address)
	}
	if t.Network.Address6 != "" {
		delete(mos.Manifest.IpAddrs, t.Network.Address6)
	}

	return nil
}
