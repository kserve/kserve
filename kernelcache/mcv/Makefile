all: mcv

##@ Help

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

### env define ###
export BIN_TIMESTAMP ?=$(shell date -u +'%Y-%m-%dT%H:%M:%SZ')
export TIMESTAMP ?=$(shell echo $(BIN_TIMESTAMP) | tr -d ':' | tr 'T' '-' | tr -d 'Z')

# restrict included verify-* targets to only process project files
SRC_ROOT           := $(shell pwd)
ARCH               := $(shell arch)
OUTPUT_DIR         := _output
BUILD_BINDIR := $(OUTPUT_DIR)/bin
GIT_VERSION        := $(shell git describe --dirty --tags --always --match='v*')
GIT_SHA            := $(shell git rev-parse HEAD)
GIT_BRANCH         := $(shell git rev-parse --abbrev-ref HEAD)
VERSION            := $(GIT_VERSION)


# set GOENV
GOOS := $(shell go env GOOS)
GOARCH := $(shell go env GOARCH)

GOENV = GO111MODULE="" \
		GOOS=$(GOOS) \
		GOARCH=$(GOARCH)
PKG_BUILD="github.com/kserve/kserve/kernelcache/mcv/pkg/build"
# Don't inherit LDFLAGS from the environment — it may contain C linker flags
# (e.g. -L/opt/homebrew/opt/libffi/lib) that are invalid for the Go linker.
LDFLAGS := \
		-X $(PKG_BUILD).Version=$(VERSION) \
		-X $(PKG_BUILD).Revision=$(GIT_SHA) \
		-X $(PKG_BUILD).Branch=$(GIT_BRANCH) \
		-X $(PKG_BUILD).OS=$(GOOS) \
		-X $(PKG_BUILD).Arch=$(GOARCH)

GPGME := gpgme

ifeq ($(shell pkg-config --exists $(GPGME) && echo 1),1)
    GPGME_FOUND := yes
else
    GPGME_FOUND := no
endif

LOCALBIN ?= $(shell pwd)/../../bin
GOLANGCI_LINT = $(LOCALBIN)/golangci-lint

BTRFS_HEADER := /usr/include/btrfs/version.h

ifneq (,$(wildcard $(BTRFS_HEADER)))
    GO_BUILD_TAGS := "$(GOOS)"
else
    GO_BUILD_TAGS := "$(GOOS),exclude_graphdriver_btrfs"
endif

ifndef GOPATH
	GOPATH := $(HOME)/go
endif

ifndef GOBIN
	GOBIN := $(GOPATH)/bin
endif

# NOTE: project related tools get installed to tmp dir which is ignored by
PROJECT_DIR := $(shell dirname $(abspath $(firstword $(MAKEFILE_LIST))))

base_dir := $(patsubst %/,%,$(dir $(realpath $(firstword $(MAKEFILE_LIST)))))

##@ Default
.PHONY: mcv
mcv: build ## Build triton cache vault.

##@ General build
.PHONY: build
build: clean_build_local _build_local  ##  Build binary and copy to $(OUTPUT_DIR)/bin

build_prereq: ## Check build prerequisites
	@test "$(GPGME_FOUND)" = "yes" || { echo "gpgme.pc not found. Install libgpgme-dev (Debian/Ubuntu) or gpgme-devel (Fedora/RHEL)."; exit 1; }
	@test "$(GOLANGCI_LINT)" != "" || { echo "golangci-lint not installed locally. run make golangci-lint from root directory."; exit 1; }
	@test "$(BTRFS_HEADER)" != "" || { echo "btrfs headers not found; excluding btrfs driver from podman storage build."; }

_build_local: build_prereq format vet ##  Build mcv binary locally.
	@echo "******     build     ******"
	@echo
	@mkdir -p "$(BUILD_BINDIR)/$(GOOS)_$(GOARCH)"
	+@$(GOENV) go build \
		-v -tags ${GO_BUILD_TAGS} \
		-ldflags "$(LDFLAGS)" \
		-o $(BUILD_BINDIR)/$(GOOS)_$(GOARCH)/mcv \
		./cmd/main.go

## toolkit ###
clean_build_local: ## Clean local build directory
	rm -rf $(BUILD_BINDIR)

format:
	@echo "******     go fmt     ******"
	@echo
	go fmt github.com/kserve/kserve/kernelcache/mcv/...
	@echo
	@echo

.PHONY: vet
vet: build_prereq ## Run go vet against code.
	@echo "******     go Vet     ******"
	@echo
	CGO_ENABLED=1 go vet -tags $(GO_BUILD_TAGS) github.com/kserve/kserve/kernelcache/mcv/...

lint: build_prereq ## Run golangci-lint against code.
	@echo "******     golangci-lint     ******"
	@echo
	CGO_ENABLED=1 $(GOLANGCI_LINT) run -c ../../.golangci.yml --build-tags $(GO_BUILD_TAGS) ./...
	@echo
	@echo

test: build_prereq ## Run unit tests.
	@echo "******     testing     ******"
	@echo
	go test -tags $(GO_BUILD_TAGS) github.com/kserve/kserve/kernelcache/mcv/...

##@ Installation
.PHONY: install
install: build ## Install mcv binary to $(GOBIN)
	@echo "Installing mcv to $(GOBIN)"
	@install -d "$(GOBIN)"
	@install "$(BUILD_BINDIR)/$(GOOS)_$(GOARCH)/mcv" "$(GOBIN)/mcv"

.PHONY: uninstall
uninstall: ## Uninstall mcv binary from $(GOBIN)
	@echo "Uninstalling mcv from $(GOBIN)"
	@rm -f "$(GOBIN)/mcv"

