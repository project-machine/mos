package provider

type ProviderType string

const (
	KVMMachineType ProviderType = "kvm"
)

type Provider interface {
	Type() ProviderType

	// Check whether a machine exists
	Exists(string) bool

	// Create a new machine
	New(mname, keyProject, UUID string) (Machine, error)

	Delete(name string) error
}

type Machine interface {
	RunProvision() error
	RunInstall() error
	Start() error
	Stop() error
}
