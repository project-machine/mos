package mosconfig

func (t Target) ValidateNetwork() bool {
	n := t.Network
	return n.Type == HostNetwork || n.Type == NoNetwork
}
