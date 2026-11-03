# The Go and Python based tools are defined in Makefile.tools.mk.
include Makefile.tools.mk

# Load dependency versions
include kserve-deps.env

# Load image configurations
include kserve-images.env

CURRENT_YEAR := $(shell date +%Y)

# Base Image URL
BASE_IMG ?= python:3.11-slim-bookworm
PMML_BASE_IMG ?= eclipse-temurin:21-jdk-noble

CRD_OPTIONS ?= "crd:maxDescLen=0"
KSERVE_ENABLE_SELF_SIGNED_CA ?= false

ENVTEST ?= $(LOCALBIN)/setup-envtest
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_VERSION ?= $(shell go list -m -f "{{ .Version }}" sigs.k8s.io/controller-runtime | awk -F'[v.]' '{printf "release-%d.%d", $$2, $$3}')
ENVTEST_K8S_VERSION ?= $(shell go list -m -f "{{ if .Replace }}{{ .Replace.Version }}{{ else }}{{ .Version }}{{ end }}" k8s.io/api | awk -F'[v.]' '{printf "1.%d", $$3}')

ENGINE ?= docker
# Empty string for local build when using podman, it allows to build different architectures
# to use do: ENGINE=podman ARCH="--arch x86_64" make docker-build-something
ARCH ?=

# CPU/Memory limits for controller-manager
KSERVE_CONTROLLER_CPU_LIMIT ?= 100m
KSERVE_CONTROLLER_MEMORY_LIMIT ?= 300Mi
$(shell perl -pi -e 's/cpu:.*/cpu: $(KSERVE_CONTROLLER_CPU_LIMIT)/' config/default/manager_resources_patch.yaml)
$(shell perl -pi -e 's/memory:.*/memory: $(KSERVE_CONTROLLER_MEMORY_LIMIT)/' config/default/manager_resources_patch.yaml)

# Force the Go toolchain defined in go.mod.
# When GOTOOLCHAIN=auto, the Go command may download a minimal toolchain to the
# module cache that is missing tools such as covdata, which breaks
# "go test -cover" (see https://go.dev/issue/75031).
# Setting GOTOOLCHAIN to the exact version from go.mod makes Go use a
# fully-installed toolchain instead.
GOTOOLCHAIN ?= auto
ifeq (auto,$(GOTOOLCHAIN))
ifeq (,$(FORCE_HOST_GO))
export GOTOOLCHAIN := $(or $(shell grep '^toolchain go' go.mod | cut -d' ' -f2),go$(shell grep '^go ' go.mod | head -1 | cut -d' ' -f2))
else
export GOTOOLCHAIN := local
endif
endif

export GOFLAGS=-mod=mod

# Go build tags (e.g. "distro" for distribution-specific code).
# Passed to Docker image builds via --build-arg and to all go commands via GOFLAGS.
GOTAGS ?=
ifdef GOTAGS
export GOFLAGS += -tags=$(GOTAGS)
endif

all: test manager agent router

include Makefile.generate.mk
include Makefile.quality.mk
include Makefile.image.mk
include Makefile.dev.mk

# Optional local/downstream overrides (ignored if absent)
-include Makefile.overrides.mk
