all: mosctl

mosctl: cmd/mosctl/*.go pkg/mosconfig/*.go
	go build ./cmd/mosctl

.PHONY: test
test: mosctl
	bats tests/install.bats
	bats tests/rfs.bats
