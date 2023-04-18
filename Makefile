DOCKER_BASE ?= docker://
UBUNTU_MIRROR ?= http://archive.ubuntu.com/ubuntu
TOP_D := $(patsubst %/,%,$(dir $(abspath $(lastword $(MAKEFILE_LIST)))))
BUILD_D = $(TOP_D)/build

BUILD_TAGS = containers_image_openpgp
TOOLSDIR := $(shell pwd)/hack/tools
PATH := bin:$(TOOLSDIR)/bin:$(PATH)
# OCI registry
ZOT := $(TOOLSDIR)/bin/zot
ZOT_VERSION := 1.4.3
# OCI registry clients
ORAS := $(TOOLSDIR)/bin/oras
ORAS_VERSION := 1.0.0-rc.1
# project-machine trust
TRUST := $(TOOLSDIR)/bin/trust
TRUST_VERSION := 0.0.3
# project-stacker stacker
STACKER := $(TOOLSDIR)/bin/stacker
STACKER_VERSION := v1.0.0-rc4

all: mosctl mosb

mosctl: mosctl.yaml cmd/mosctl/*.go pkg/mosconfig/*.go $(STACKER)
	$(STACKER) --storage-type=overlay \
	"--oci-dir=$(BUILD_D)/oci" "--roots-dir=$(BUILD_D)/roots" "--stacker-dir=$(BUILD_D)/stacker" \
	build --shell-fail \
	"--substitute=TOP_D=$(TOP_D)" \
	"--substitute=DOCKER_BASE=$(DOCKER_BASE)" \
	"--substitute=UBUNTU_MIRROR=$(UBUNTU_MIRROR)" \
	"--substitute=BUILD_TAGS=$(BUILD_TAGS)" \
	"--layer-type=tar" \
	"--stacker-file=mosctl.yaml"

mosb: mosb.yaml cmd/mosb/*.go pkg/mosconfig/*.go $(STACKER)
	$(STACKER) --storage-type=overlay \
	"--oci-dir=$(BUILD_D)/oci" "--roots-dir=$(BUILD_D)/roots" "--stacker-dir=$(BUILD_D)/stacker" \
	build --shell-fail \
	"--substitute=TOP_D=$(TOP_D)" \
	"--substitute=DOCKER_BASE=$(DOCKER_BASE)" \
	"--substitute=UBUNTU_MIRROR=$(UBUNTU_MIRROR)" \
	"--substitute=BUILD_TAGS=$(BUILD_TAGS)" \
	"--layer-type=tar" \
	"--stacker-file=mosb.yaml"

$(STACKER):
	mkdir -p $(TOOLSDIR)/bin
	curl -Lo $(STACKER) https://github.com/project-stacker/stacker/releases/download/$(STACKER_VERSION)/stacker
	chmod +x $(STACKER)

$(ZOT):
	mkdir -p $(TOOLSDIR)/bin
	curl -Lo $(ZOT) https://github.com/project-zot/zot/releases/download/v$(ZOT_VERSION)/zot-linux-amd64-minimal
	chmod +x $(ZOT)

$(TRUST):
	mkdir -p $(TOOLSDIR)/bin
	curl -Lo $(TRUST) https://github.com/project-machine/trust/releases/download/${TRUST_VERSION}/trust
	chmod +x $(TRUST)

$(ORAS):
	mkdir -p $(TOOLSDIR)/bin
	curl -Lo oras.tar.gz https://github.com/oras-project/oras/releases/download/v$(ORAS_VERSION)/oras_$(ORAS_VERSION)_linux_amd64.tar.gz
	tar xvzf oras.tar.gz -C $(TOOLSDIR)/bin oras
	rm oras.tar.gz

.PHONY: test
test: mosctl mosb $(ORAS) $(ZOT) $(TRUST)
	bats tests/install.bats
	bats tests/rfs.bats
	bats tests/activate.bats
	bats tests/update.bats
	bats tests/mount.bats

.PHONY: clean
clean:
	rm -f mosb mosctl

.PHONY: dist-clean
dist-clean: clean
	rm -rf $(TOOLSDIR)
	lxc-usernsexec -s -- rm -Rf "$(TOP_D)/build"
