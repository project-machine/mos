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

GO_SRC=$(shell find cmd pkg  -name "*.go")

all: mosctl mosb $(ZOT) $(ORAS)

mosctl: .made-gofmt $(GO_SRC)
	go build -tags "$(BUILD_TAGS)" -ldflags "-s -w" ./cmd/mosctl

mosb: .made-gofmt $(GO_SRC)
	go build -tags "$(BUILD_TAGS)" -ldflags "-s -w" ./cmd/mosb

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

.PHONY: gofmt
gofmt: .made-gofmt

.made-gofmt: $(GO_SRC)
	@o=$$(gofmt -l -w . 2>&1) || \
	  { r=$$?; echo "gofmt failed [$$r]: $$o" 1>&2; exit $$r; }; \
	  [ -z "$$o" ] || { echo "gofmt made changes: $$o" 1>&2; exit 1; }
	@touch $@

.PHONY: test
test: mosctl mosb $(ORAS) $(ZOT) $(TRUST)
	bats tests/install.bats
	bats tests/rfs.bats
	bats tests/activate.bats
	bats tests/update.bats
	bats tests/mount.bats

clean:
	rm -f mosb mosctl
	rm -rf $(TOOLSDIR)
