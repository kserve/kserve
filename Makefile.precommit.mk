# Makefile.precommit.mk - Smart precommit orchestration
#
# This file owns `make precommit`, `make precommit-fast`, and `make check`.
# It parallelizes independent work and optionally skips expensive targets
# when irrelevant files changed.
#
# Quick reference:
#   make precommit                 Run all targets (backward compatible)
#   make precommit-fast            Run only targets whose trigger paths have changes
#   make precommit-fast FORCE_FULL=1  Force all conditional targets in fast mode
#   make check                     CI target: precommit + verify clean tree
#
# ---
#
# How it works
#
# Step 1 - always runs:
#   tidy (runs first - writes go.mod, must complete before parallel sync)
#
# Step 2 - always runs, parallel:
#   precommit-sync   (ensure-go-version-upgrade, sync-deps, sync-img-env)
#   precommit-lint   (go-lint)
#   fetch-external-crds (cached by version sentinel)
#
# Step 3 - parallel, conditional generation:
#   vet + generate
#
# Step 4 - parallel, after generate:
#   py-fmt + py-lint + manifests + uv-lock
#   py-fmt/py-lint must run after generate (client-gen writes python/kserve/).
#   manifests must run after generate (both write zz_generated.deepcopy.go).
#
# Step 5 - sequential, conditional:
#   sync-helm-* - generate-chart-manifests - helm-docs-gen + install scripts
#   (skipped entirely if no config/charts/setup files changed)
#
# Change detection (precommit-fast only): targets run only when their trigger
# paths overlap with dirty files. FORCE_FULL=1 overrides detection.
#
# ---
#
# Adding a new validation target
#
# 1. Define your target in the main Makefile (or any included file).
#
# 2. Pick the right step:
#    - Step 2: sync/mutation work that later steps depend on
#    - Steps 3-4: checks and generation that can run in parallel
#    - Step 5: work that depends on step 4 output (e.g. chart generation)
#
# 3. Should it always run or only when relevant files change?
#
#    Always run - add it to the appropriate parallel step in precommit.
#
#    Conditionally run - define a trigger variable and wire it in:
#      YOURTARGET_TRIGGERS = path/to/watch/ another/path/
#      Then add to the appropriate step:
#        $(call run-if-changed,your-target,$(YOURTARGET_TRIGGERS))
#
# 4. Ordering constraints? If your target writes files another target reads,
#    put them in separate sequential steps (like generate before manifests).
#
# ---

## Change detection triggers
#
# Each variable lists git pathspecs that should cause the associated target
# to run. Directory prefixes match recursively. Use :(glob) for patterns
# (quote the pathspec to prevent shell from interpreting parentheses).

GENERATE_TRIGGERS = pkg/apis/serving/ hack/update-codegen.sh \
  hack/update-openapigen.sh hack/python-sdk/ cmd/spec-gen/ \
  go.mod hack/boilerplate.go.txt \
  pkg/openapi/

MANIFESTS_TRIGGERS = pkg/apis/serving/ pkg/controller/ config/crd/ \
  config/rbac/ config/configmap/ hack/minimal-crdgen.sh \
  cmd/crd-gen/ kserve-deps.env Makefile

UVLOCK_TRIGGERS = ':(glob)python/*/pyproject.toml'

PHASE2_TRIGGERS = config/ charts/ hack/setup/ kserve-deps.env \
  kserve-images.env

## Force full run (truthy value; 0 and false disable)
FORCE_FULL ?=
ifneq ($(filter 0 false,$(FORCE_FULL)),)
  override FORCE_FULL :=
endif

## run-if-changed macro
#
# Returns the target name when it should run, empty string to skip.
# Evaluated at Make parse time via $(shell).
#
# Two modes:
#   FORCE_FULL set - always returns target (run everything)
#   Otherwise      - returns target only if trigger paths have changes
#
# Usage: $(call run-if-changed,TARGET_NAME,TRIGGER_PATHSPECS)
ifneq ($(FORCE_FULL),)
  run-if-changed = $(1)
else
  run-if-changed = $(shell hack/changed-paths.sh $(2) && echo $(1))
endif

## Fetch and cache an external CRD by version
#
# Re-fetches only when the version changes or the output file is missing.
# Sentinel files (test/crds/.<id>-version) track the cached version.
# The sentinel is only written after a successful fetch - a failed kustomize
# won't leave a stale sentinel or empty CRD file.
#
# $(1) = identifier   $(2) = version   $(3) = kustomize URL   $(4) = output file
define fetch-cached-crd
	@if [ ! -f test/crds/.$(1)-version ] || \
	    [ "$$(cat test/crds/.$(1)-version)" != "$(2)" ] || \
	    [ ! -f "$(4)" ]; then \
	  echo "==> Fetching $(1) CRD $(2)..."; \
	  $(KUSTOMIZE) build "$(3)" > "$(4).tmp" || \
	    { echo "ERROR: failed to fetch $(1) CRD from $(3)"; rm -f "$(4).tmp"; exit 1; }; \
	  mv "$(4).tmp" "$(4)"; \
	  echo "$(2)" > test/crds/.$(1)-version; \
	else \
	  echo "==> $(1) CRD cached ($(2))"; \
	fi
endef

.PHONY: fetch-external-crds
fetch-external-crds: kustomize
	$(call fetch-cached-crd,gie,$(GIE_VERSION),https://github.com/kubernetes-sigs/gateway-api-inference-extension.git/config/crd?ref=$(GIE_VERSION),config/llmisvc/gateway-inference-extension.yaml)
	@cp config/llmisvc/gateway-inference-extension.yaml test/crds/
	$(call fetch-cached-crd,wva,$(WVA_VERSION),https://github.com/llm-d/llm-d-workload-variant-autoscaler.git/config/crd?ref=$(WVA_VERSION),test/crds/wva_variantautoscalings.yaml)

## helm-docs as a standalone target (moved from generate for Phase 2)
.PHONY: helm-docs-gen
helm-docs-gen: helm-docs
	@$(HELM_DOCS) --chart-search-root=charts --output-file=README.md

## Precommit orchestration

# Selective targets for precommit-fast, stripped so empty list produces empty string.
_FAST_GENERATE := $(strip $(call run-if-changed,generate,$(GENERATE_TRIGGERS)))
_FAST_MANIFESTS_UVLOCK := $(strip $(call run-if-changed,manifests,$(MANIFESTS_TRIGGERS)) \
  $(call run-if-changed,uv-lock,$(UVLOCK_TRIGGERS)))

# Common steps shared by precommit and precommit-fast.
# Runs tidy first (writes go.mod), then sync + lint + CRDs in parallel.
define _precommit-common
	@echo "==> tidy"
	@$(MAKE) --no-print-directory tidy
	@echo "==> sync, lint, CRD cache"
	@$(MAKE) --no-print-directory -j \
	  precommit-sync precommit-lint fetch-external-crds
endef

.PHONY: precommit precommit-fast precommit-sync precommit-lint
precommit:
	$(_precommit-common)
	@echo "==> vet, generate"
	@$(MAKE) --no-print-directory -j vet generate
	@echo "==> format, lint, manifests, uv-lock"
	@$(MAKE) --no-print-directory -j py-fmt py-lint manifests uv-lock
	@echo "==> charts, install scripts"
	@$(MAKE) --no-print-directory sync-helm-common-helpers \
	  sync-helm-common-resource-helpers sync-helm-multi-resource-helpers
	@$(MAKE) --no-print-directory generate-chart-manifests
	@$(MAKE) --no-print-directory helm-docs-gen generate-quick-install-scripts

precommit-fast:
	$(_precommit-common)
	@echo "==> vet, generate"
	@$(MAKE) --no-print-directory -j vet $(_FAST_GENERATE)
	@echo "==> format, lint, manifests, uv-lock"
	@$(MAKE) --no-print-directory -j py-fmt py-lint $(_FAST_MANIFESTS_UVLOCK)
	@echo "==> charts, install scripts"
	@if hack/changed-paths.sh $(PHASE2_TRIGGERS); then \
	  $(MAKE) --no-print-directory sync-helm-common-helpers \
	    sync-helm-common-resource-helpers sync-helm-multi-resource-helpers && \
	  $(MAKE) --no-print-directory generate-chart-manifests && \
	  $(MAKE) --no-print-directory helm-docs-gen generate-quick-install-scripts; \
	fi

precommit-sync: ensure-go-version-upgrade sync-deps sync-img-env

precommit-lint:
	@$(MAKE) --no-print-directory go-lint

## CI target - runs precommit then verifies no uncommitted changes remain.
.PHONY: check
check: precommit
	@if [ -n "$$(git status -s)" ]; then \
		echo "The following differences will fail CI until committed:"; \
		git diff --exit-code; \
		echo "Please ensure that you have run 'make precommit' and committed the changes."; \
		exit 1; \
	fi
