BUILD_TAGS = containers_image_openpgp
TOOLSDIR := $(shell pwd)/hack/tools
PATH := bin:$(TOOLSDIR)/bin:$(PATH)
# OCI registry
ZOT := $(TOOLSDIR)/bin/zot
ZOT_VERSION := 2.0.0-rc5
# OCI registry clients
ORAS := $(TOOLSDIR)/bin/oras
ORAS_VERSION := 1.0.0-rc.1
REGCTL := $(TOOLSDIR)/bin/regctl
REGCTL_VERSION := 0.5.0
STACKER := $(TOOLSDIR)/bin/stacker
STACKER_VERSION := v1.0.0-rc8
TOPDIR := $(shell git rev-parse --show-toplevel)
#BOOTKIT_VERSION ?= "v0.0.17.231018"
# We need a bootkit with new mosctl which knows about the new network.
# So use a manually built one.  Then we can auot-build new bootkit,
# then update the below to the real v0.0.18.2311XX version.
BOOTKIT_VERSION ?= "v0.0.18.tmp"
ROOTFS_VERSION = $(BOOTKIT_VERSION)

archout = $(shell arch)
ifeq ("$(archout)", "aarch64")
arch = arm64
else ifeq ("$(archout)", "x86_64")
arch = amd64
else
#error "Unsupported architecture: $(archout)"
endif

MAIN_VERSION ?= $(shell git describe --always --dirty || echo no-git)
ifeq ($(MAIN_VERSION),$(filter $(MAIN_VERSION), "", no-git))
$(error "Bad value for MAIN_VERSION: '$(MAIN_VERSION)'")
endif

GO_SRC=$(shell find cmd pkg  -name "*.go")

all: mosctl mosb trust $(ZOT) $(ORAS) $(REGCTL)

VERSION_LDFLAGS=-X github.com/project-machine/mos/pkg/mosconfig.Version=$(MAIN_VERSION) \
	-X github.com/project-machine/mos/pkg/trust.Version=$(MAIN_VERSION) \
	-X github.com/project-machine/mos/pkg/mosconfig.LayerVersion=0.0.4 \
	-X github.com/project-machine/mos/pkg/trust.BootkitVersion=$(BOOTKIT_VERSION)

mosctl: .made-gofmt $(GO_SRC)
	go build -tags "$(BUILD_TAGS)" -ldflags "-s -w $(VERSION_LDFLAGS)" ./cmd/mosctl

mosb: .made-gofmt $(GO_SRC)
	go build -tags "$(BUILD_TAGS)" -ldflags "-s -w $(VERSION_LDFLAGS)" ./cmd/mosb

trust: .made-gofmt $(GO_SRC)
	go build -tags "$(BUILD_TAGS)" -ldflags "-s -w $(VERSION_LDFLAGS)" ./cmd/trust

$(ZOT):
	mkdir -p $(TOOLSDIR)/bin
	curl -Lo $(ZOT) https://github.com/project-zot/zot/releases/download/v$(ZOT_VERSION)/zot-linux-${arch}-minimal
	chmod +x $(ZOT)

$(ORAS):
	mkdir -p $(TOOLSDIR)/bin
	curl -Lo oras.tar.gz https://github.com/oras-project/oras/releases/download/v$(ORAS_VERSION)/oras_$(ORAS_VERSION)_linux_$(arch).tar.gz
	tar xvzf oras.tar.gz -C $(TOOLSDIR)/bin oras
	rm oras.tar.gz

$(REGCTL):
	mkdir -p $(TOOLSDIR)/bin
	curl -Lo $(REGCTL) https://github.com/regclient/regclient/releases/download/v$(REGCTL_VERSION)/regctl-linux-$(arch)
	chmod +x $(REGCTL)

$(STACKER):
	mkdir -p $(TOOLSDIR)/bin
	curl -Lo $(STACKER) https://github.com/project-stacker/stacker/releases/download/$(STACKER_VERSION)/stacker
	chmod +x $(STACKER)

.PHONY: gofmt
gofmt: .made-gofmt

.made-gofmt: $(GO_SRC)
	@o=$$(gofmt -l -w cmd pkg 2>&1) || \
	  { r=$$?; echo "gofmt failed [$$r]: $$o" 1>&2; exit $$r; }; \
	  [ -z "$$o" ] || { echo "gofmt made changes: $$o" 1>&2; exit 1; }
	@touch $@

deps: mosctl mosb trust $(ORAS) $(REGCTL) $(ZOT)

STACKER_SUBS = \
	--substitute ROOTFS_VERSION=$(BOOTKIT_VERSION) \
	--substitute TOPDIR=${TOPDIR} \
	--substitute ZOT_VERSION=$(ZOT_VERSION)

STACKER_OPTS = --layer-type=squashfs $(STACKER_SUBS)

.PHONY: layers
layers: mosctl $(STACKER)
	$(STACKER) build $(STACKER_OPTS) --stacker-file layers/provision/stacker.yaml
	$(STACKER) build $(STACKER_OPTS) --stacker-file layers/install/stacker.yaml

.PHONY: publish
publish: layers
	$(STACKER) publish $(STACKER_OPTS) --stacker-file layers/provision/stacker.yaml --url $(URL) --username $(USERNAME) --password $(PASSWORD)

.PHONY: test
test: deps
	rm -rf ~/.local/share/machine.fortests
	bats tests/install.bats
	bats tests/rfs.bats
	bats tests/activate.bats
	bats tests/update.bats
	bats tests/mount.bats
	bats tests/keyset.bats
	bats tests/launch.bats

# the trust testcases only, for running on amd64.  We need an arm64
# runner capable of doing nested virt if we're going to have github
# actions run the mos tests for arm64, and we don't have that.  Yet.
.PHONY: test-trust
test-trust: trust
	bats tests/keyset.bats

clean:
	rm -f mosb mosctl trust
	rm -rf $(TOOLSDIR)
	stacker clean
