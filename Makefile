all: mosctl

mosctl: cmd/mosctl/*.go
	go build ./...

.PHONY: test
test: mosctl
	bats tests/install.bats
