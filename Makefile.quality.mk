# Quality gates: format, lint, vet, verify, test, precommit, release prep.

##@ Quality gates and tests

.PHONY: setup-envtest
setup-envtest: envtest ## Before plain go test on envtest suites: fetch envtest binaries, print their path.
	@echo "Setting up envtest binaries for Kubernetes version $(ENVTEST_K8S_VERSION)..."
	@$(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path || { \
		echo "Error: Failed to set up envtest binaries for version $(ENVTEST_K8S_VERSION)."; \
		exit 1; \
		}

fmt: ## go fmt the three Go modules (root, qpext, kernelcache/mcv). Rewrites files.
	@for dir in . qpext kernelcache/mcv; do \
	  echo "Formatting $$dir..."; \
	  (cd $$dir && go fmt ./...) || exit 1; \
	done

py-fmt: $(RUFF) ## ruff format python/, docs/, test/ and hack/. Rewrites files.
	$(RUFF) format --config ruff.toml ./python ./docs ./test ./hack

vet: ## Compile and vet Go code, tests included: pkg/, cmd/, qpext. kernelcache/mcv needs gpgme.
	go vet ./pkg/... ./cmd/... && cd qpext && go vet ./...
	@if pkg-config --exists gpgme 2>/dev/null; then \
		echo "Vetting kernelcache/mcv..."; \
		(cd kernelcache/mcv && CGO_ENABLED=1 go vet ./...) || exit 1; \
	else \
		echo "Skipping kernelcache/mcv vet (CGO dependencies not installed, covered by mcv-build-test CI)"; \
	fi

tidy: ## After changing Go dependencies: go mod tidy in the three Go modules.
	@for dir in . qpext kernelcache/mcv; do \
	  echo "Tidying $$dir..."; \
	  (cd $$dir && go mod tidy) || exit 1; \
	done

go-lint: golangci-lint ## golangci-lint --fix on the Go modules. Rewrites files.
	@for dir in . qpext; do \
	  echo "Linting $$dir..."; \
	  (cd $$dir && $(GOLANGCI_LINT) run --fix) || exit 1; \
	done
	@if pkg-config --exists gpgme 2>/dev/null; then \
		echo "Linting kernelcache/mcv..."; \
		(cd kernelcache/mcv && CGO_ENABLED=1 $(GOLANGCI_LINT) run --fix) || exit 1; \
	else \
		echo "Skipping kernelcache/mcv lint (CGO dependencies not installed, covered by mcv-build-test CI)"; \
	fi

py-lint: $(RUFF) ## ruff check the Python code. Read-only.
	$(RUFF) check --config ruff.toml

# Shell scripts to lint. Expressed as exclusions so a newly added script is
# linted by default instead of having to be opted in:
#   install/**                    - frozen per-release install snapshots, never edited after a release is cut
#   hack/setup/quick-install/*.sh - written by `make generate-quick-install-scripts`
# Using `git ls-files` also means gitignored output (bin/, venvs, ...) is skipped for free.
SHELLCHECK_EXCLUDES := ':(exclude)install/**' ':(exclude)hack/setup/quick-install/*.sh'

# Gate at `error`, the severity the tree is already clean at. Tightening to
# `warning` is a bounded follow-up (mostly SC2155/SC2206/SC2034); run
# `make sh-lint SHELLCHECK_SEVERITY=warning` to see that backlog.
SHELLCHECK_SEVERITY ?= error
# ShellCheck is CPU-bound per script; keep parallelism bounded for laptops and CI.
SHELLCHECK_JOBS ?= 4
SHELLCHECK_BATCH_SIZE ?= 8

.PHONY: sh-lint
sh-lint: $(SHELLCHECK) ## After editing shell scripts: shellcheck at SHELLCHECK_SEVERITY (default error).
	@echo "Shell-linting scripts (severity: $(SHELLCHECK_SEVERITY))"
	@git ls-files -z --cached --others --exclude-standard -- '*.sh' $(SHELLCHECK_EXCLUDES) \
		| xargs -0 -r -n $(SHELLCHECK_BATCH_SIZE) -P $(SHELLCHECK_JOBS) $(SHELLCHECK) --severity=$(SHELLCHECK_SEVERITY)

e2e-collect: $(PYTEST) ## After editing test/e2e: check pytest collects it. Installs python/kserve in bin/.venv.
	$(UV) pip install --python $(PYTHON_BIN)/python -e ./python/kserve -q
	$(UV) pip install --python $(PYTHON_BIN)/python --group test --directory ./python/kserve -q
	$(PYTEST) --collect-only test/e2e/ -q

pin-actions: pinact ## After adding or bumping a GitHub Action: pin it to a commit SHA. Needs network.
	GITHUB_TOKEN=$$(gh auth token 2>/dev/null) $(PINACT) run .github/workflows/*.yml .github/workflows/*.yaml

# Verify that all GitHub Actions are pinned to a full-length commit SHA (offline check, no API calls).
verify-pinned-actions:
	@if grep -rPn 'uses:\s+\S+@(?!([0-9a-f]{40}))' .github/workflows/*.yml .github/workflows/*.yaml 2>/dev/null; then \
		echo "ERROR: Found GitHub Actions not pinned to a full SHA. Run 'make pin-actions' to fix."; \
		exit 1; \
	fi

validate-infra-scripts:
	@python3 hack/setup/scripts/validate-install-scripts.py

lint-helm-charts:
	@bash hack/setup/scripts/lint-helm.sh

verify-helm-helpers-consistency:
	@bash hack/setup/scripts/verify-helm-helpers.sh

verify-helm-storage-overrides:
	@bash hack/setup/scripts/verify-helm-storage-overrides.sh

verify-minimal-crd-sync:
	@bash hack/verify-minimal-crd-sync.sh

.PHONY: ensure-go-version-upgrade ensure-golangci-go-version
ensure-go-version-upgrade: ensure-golangci-go-version

ensure-golangci-go-version: yq	
	@GO_GOMOD_VERSION="$$(grep -m1 '^go ' go.mod | cut -d' ' -f2 | cut -d. -f1-2)"; \
	GO_GOLANGCI_VERSION="$$($(YQ) -r '.run.go // ""' .golangci.yml | cut -d. -f1-2)"; \
	if [ -z "$${GO_GOLANGCI_VERSION}" ]; then \
		echo "INFO: '.golangci.yml:run.go' is not set; defaulting to $$GO_GOMOD_VERSION."; \
		GO_GOLANGCI_VERSION="$${GO_GOMOD_VERSION}"; \
	fi; \
	if [ "$${GO_GOMOD_VERSION}" != "$${GO_GOLANGCI_VERSION}" ]; then \
		echo "ERROR: go.mod uses Go $$GO_GOMOD_VERSION but .golangci.yml uses $$GO_GOLANGCI_VERSION"; \
		echo "Please update '.golangci.yml:run.go' to $$GO_GOMOD_VERSION (major.minor) and rerun 'make precommit'."; \
		exit 1; \
	fi
# Sync common helpers to all charts (must run before helm package)
sync-helm-common-helpers:
	@echo "Syncing common helpers to all charts..."
	@for chart in kserve-resources kserve-llmisvc-resources kserve-localmodel-resources kserve-runtime-configs; do \
		cp charts/_common/_utils.tpl charts/$$chart/templates/_utils.tpl; \
		echo "  ✓ Copied to charts/$$chart/templates/_utils.tpl"; \
	done

# Configuration: in setup.cfg [codespell] section (skip paths, ignore-words-list)
spell-check: $(CODESPELL) ## codespell the repo. Read-only.
	$(CODESPELL)

precommit: ensure-go-version-upgrade sync-deps sync-img-env vet go-lint py-fmt py-lint sh-lint e2e-collect generate tidy manifests uv-lock generate-quick-install-scripts generate-chart-manifests sync-helm-common-helpers sync-helm-common-resource-helpers sync-helm-multi-resource-helpers verify-pinned-actions verify-minimal-crd-sync boilerplate spell-check ## Before each commit: lint --fix, every generator, lock files. Rewrites files, needs network.

check: precommit ## What CI runs: precommit, then fail if it left the tree dirty.
	@if [ ! -z "`git status -s`" ]; then \
		echo "The following differences will fail CI until committed:"; \
		git diff --exit-code; \
		echo "Please ensure that you have run 'make precommit' and committed the changes."; \
		exit 1; \
	fi

# Override TEST_PKGS to focus on specific packages, e.g.:
#   make test TEST_PKGS="./pkg/controller/v1alpha2/llmisvc/..."
TEST_PKGS ?= $$(go list ./pkg/...) ./cmd/...
TEST_TIMEOUT ?= 30m
# -vet=off: go test's built-in vet runs a subset of the analyzers `make vet` runs,
# and `make vet` already ran - as a prerequisite of test, or in precommit-check on CI.
GO_TEST = KUBEBUILDER_ASSETS="$$($(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test -vet=off --timeout $(TEST_TIMEOUT) $(TEST_PKGS) -coverprofile coverage.out -coverpkg ./pkg/... ./cmd...
test: fmt vet manifests envtest test-qpext ## Go tests (pkg/, cmd/, qpext). Runs fmt, vet, manifests first: rewrites files, needs network.
	$(GO_TEST)

# The Go Test workflow runs this: precommit-check already fails on fmt, vet and
# manifests drift, so repeating them there only costs time.
.PHONY: test-only
test-only: envtest test-qpext ## After precommit, and in CI: test without its fmt, vet, manifests steps. Needs network.
	$(GO_TEST)

# Run a subset of the Go tests against the envtest assets of `make test`, without
# its fmt, vet, manifests and qpext steps. Fails when no test or spec ran.
#   make test-focus PKG=./pkg/controller/v1alpha2/llmisvc RUN=TestLLMInferenceServiceController FOCUS='<spec text>'
# PKG takes package patterns, RUN a `go test -run` regex, FOCUS a Ginkgo focus regex.
# RUN and FOCUS are exported unexpanded: make would otherwise read `$|` or `$/`
# in a regex as a variable reference and pass a different pattern.
.PHONY: test-focus
test-focus: export override RUN := $(value RUN)
test-focus: export override FOCUS := $(value FOCUS)
test-focus: envtest ## While iterating: run PKG=, RUN=, FOCUS= without fmt, vet, manifests. Fails if nothing ran.
	@KUBEBUILDER_ASSETS="$$($(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" \
		TEST_TIMEOUT=$(TEST_TIMEOUT) hack/test-focus.sh

test-qpext: ## After qpext/ changes: run only the qpext module tests.
	cd qpext && go test -v ./... -cover

bump-version: ## Release prep: bump versions, uv-lock, precommit, install/. Set PRIOR_VERSION, NEW_VERSION.
	@echo "bumping version numbers for this release"
	@hack/release/prepare-for-release.sh $(PRIOR_VERSION) $(NEW_VERSION)

.PHONY: check-doc-links
check-doc-links: ## After editing Markdown: check that links resolve. Needs network.
	@python3 hack/verify-doc-links.py && echo "$@: OK"

# Kept out of precommit: it pulls and runs every image whose tag changed versus BASE_REF.
.PHONY: check-image-flag-drift
check-image-flag-drift: yq ## Image tag bump: replay manifest flags on changed images. Needs a container engine, network.
	@BASE_REF=$(or $(BASE_REF),origin/master) HEAD_REF=$(or $(HEAD_REF),HEAD) \
		ENGINE=$(ENGINE) YQ=$(YQ) \
		hack/verify-image-flag-drift.sh && echo "$@: OK"
