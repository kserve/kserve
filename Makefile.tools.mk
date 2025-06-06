LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)
PYTHON_VENV = $(LOCALBIN)/.venv
PYTHON_BIN = $(PYTHON_VENV)/bin


## Tool binary names.
GOLANGCI_LINT = $(LOCALBIN)/golangci-lint
CONTROLLER_GEN = $(LOCALBIN)/controller-gen
ENVTEST = $(LOCALBIN)/setup-envtest
YQ = $(LOCALBIN)/yq
HELM_DOCS = $(LOCALBIN)/helm-docs
BLACK_FMT = $(PYTHON_BIN)/black
FLAKE8_LINT = $(PYTHON_BIN)/flake8
POETRY = $(PYTHON_BIN)/poetry

## Tool versions.
GOLANGCI_LINT_VERSION ?= v1.64.8
CONTROLLER_TOOLS_VERSION ?= v0.16.2
ENVTEST_VERSION ?= latest
YQ_VERSION ?= v4.28.1
HELM_DOCS_VERSION ?= v1.12.0
BLACK_FMT_VERSION ?= 24.3
FLAKE8_LINT_VERSION ?= 7.1
POETRY_VERSION ?= 1.8.3



.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT)
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))

## Download controller-gen locally if necessary.
.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN)
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_TOOLS_VERSION))

## Download envtest-setup locally if necessary.
.PHONY: envtest
envtest: $(ENVTEST)
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,$(ENVTEST_VERSION))

## Download yq locally if necessary.
.PHONY: yq
yq: $(YQ)
$(YQ): $(LOCALBIN)
	$(call go-install-tool,$(YQ),github.com/mikefarah/yq/v4,$(YQ_VERSION))

## Download helm-docs locally if necessary.
.PHONY: helm-docs
helm-docs: $(HELM_DOCS)
$(HELM_DOCS): $(LOCALBIN)
	$(call go-install-tool,$(HELM_DOCS),github.com/norwoodj/helm-docs/cmd/helm-docs,$(HELM_DOCS_VERSION))

$(PYTHON_VENV): | $(LOCALBIN)
	python3 -m venv $(PYTHON_VENV)
	$(PYTHON_BIN)/pip install --upgrade pip

$(BLACK_FMT) $(FLAKE8_LINT): $(PYTHON_VENV)
	$(PYTHON_BIN)/pip install black==$(BLACK_FMT_VERSION) flake8==$(FLAKE8_LINT_VERSION)

$(POETRY): $(PYTHON_VENV)
	$(PYTHON_BIN)/pip install poetry==$(POETRY_VERSION) python/plugin/poetry-version-plugin

# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary
# $2 - package url which can be installed
# $3 - specific version of package
define go-install-tool
@[ -f "$(1)-$(3)" ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
rm -f $(1) || true ;\
GOBIN=$(LOCALBIN) go install $${package} ;\
go mod tidy ;\
mv $(1) $(1)-$(3) ;\
} ;\
ln -sf $(1)-$(3) $(1)
endef
