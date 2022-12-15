all: mosctl

mosctl: cmd/mosctl/*.go
	go build ./cmd/mosctl

.PHONY: test
test: mosctl
	bats tests/install.bats
