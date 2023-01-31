all: mosctl mosb

mosctl: cmd/mosctl/*.go pkg/mosconfig/*.go
	go build ./cmd/mosctl

mosb: cmd/mosb/*.go pkg/mosconfig/*.go
	go build ./cmd/mosb

.PHONY: test
test: mosctl
	bats tests/install.bats
	bats tests/rfs.bats
	bats tests/soci.bats
	bats tests/activate.bats
	bats tests/lxc.bats
	bats tests/update.bats

clean:
	rm -f mosb mosctl
