BUILD_TAGS = containers_image_openpgp
TOOLSDIR := $(shell pwd)/hack/tools
PATH := bin:$(TOOLSDIR)/bin:$(PATH)
# OCI registry
ZOT := $(TOOLSDIR)/bin/zot
ZOT_VERSION := 1.4.3
# OCI registry clients
ORAS := $(TOOLSDIR)/bin/oras
ORAS_VERSION := 1.0.0-rc.1

all: mosctl mosb $(ZOT) $(ORAS)

mosctl: cmd/mosctl/*.go pkg/mosconfig/*.go
	go build -tags "$(BUILD_TAGS)" ./cmd/mosctl

mosb: cmd/mosb/*.go pkg/mosconfig/*.go
	go build -tags "$(BUILD_TAGS)" ./cmd/mosb

$(ZOT):
	mkdir -p $(TOOLSDIR)/bin
	curl -Lo $(ZOT) https://github.com/project-zot/zot/releases/download/v$(ZOT_VERSION)/zot-linux-amd64-minimal
	chmod +x $(ZOT)

$(ORAS):
	mkdir -p $(TOOLSDIR)/bin
	curl -Lo oras.tar.gz https://github.com/oras-project/oras/releases/download/v$(ORAS_VERSION)/oras_$(ORAS_VERSION)_linux_amd64.tar.gz
	tar xvzf oras.tar.gz -C $(TOOLSDIR)/bin oras
	rm oras.tar.gz

.PHONY: test
test: mosctl mosb $(ORAS) $(ZOT)
	bats tests/install.bats
	bats tests/rfs.bats
	bats tests/soci.bats
	bats tests/activate.bats
	bats tests/update.bats

clean:
	rm -f mosb mosctl
	rm -rf $(TOOLSDIR)
