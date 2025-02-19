LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool binary names.
GOLANGCI_LINT = $(LOCALBIN)/golangci-lint
CONTROLLER_GEN = $(LOCALBIN)/controller-gen
ENVTEST = $(LOCALBIN)/setup-envtest
YQ = $(LOCALBIN)/yq
BLACK_FMT = $(LOCALBIN)/.venv/black@v24.3/bin/black
FLAKE8_LINT = $(LOCALBIN)/.venv/flake8@v7.1/bin/flake8

## Tool versions.
GOLANGCI_LINT_VERSION ?= v1.63
CONTROLLER_TOOLS_VERSION ?= v0.16.2
ENVTEST_VERSION ?= latest
YQ_VERSION ?= v4.28.1


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

$(BLACK_FMT): $(LOCALBIN)/.venv/black@v24.3

$(FLAKE8_LINT): $(LOCALBIN)/.venv/flake8@v7.1


$(LOCALBIN)/.venv/%:
	mkdir -p $(@D)
	python3 -m venv $@
	$@/bin/pip3 install $$(echo $* | sed 's/@/==/')

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
mv $(1) $(1)-$(3) ;\
} ;\
ln -sf $(1)-$(3) $(1)
endef