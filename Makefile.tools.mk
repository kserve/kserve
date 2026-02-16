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
UV = $(PYTHON_BIN)/uv
RUFF = $(PYTHON_BIN)/ruff

## Tool versions are defined in kserve-deps.env (included in main Makefile)

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
	@[ -f "$(YQ)-$(YQ_VERSION)" ] || { \
	BIN_DIR=$(LOCALBIN) hack/setup/cli/install-yq.sh && \
	mv $(LOCALBIN)/yq $(YQ)-$(YQ_VERSION) ; \
	} ; \
	ln -sf "$$(basename $(YQ)-$(YQ_VERSION))" "$(YQ)"

## Download helm-docs locally if necessary.
.PHONY: helm-docs
helm-docs: $(HELM_DOCS)
$(HELM_DOCS): $(LOCALBIN)
	$(call go-install-tool,$(HELM_DOCS),github.com/norwoodj/helm-docs/cmd/helm-docs,$(HELM_DOCS_VERSION))

$(PYTHON_VENV): | $(LOCALBIN)
	python3 -m venv $(PYTHON_VENV)
	$(PYTHON_BIN)/pip install --upgrade pip

$(BLACK_FMT): $(PYTHON_VENV)
	$(PYTHON_BIN)/pip install black==$(BLACK_FMT_VERSION)

$(UV): $(PYTHON_VENV)
	$(PYTHON_BIN)/pip install uv==$(UV_VERSION)

$(RUFF): $(PYTHON_VENV)
	$(PYTHON_BIN)/pip install ruff==$(RUFF_VERSION)

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
