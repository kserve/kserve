# Midstream-only Make targets for opendatahub-io/kserve.
# Loaded via `-include Makefile.overrides.mk` in the main Makefile.
# This file does not exist on upstream kserve/kserve.

# UBI base image for Python Dockerfiles (upstream defaults to python:3.11-slim-bookworm).
BASE_IMG = registry.access.redhat.com/ubi9/python-311:9.7

# Enable distro build tag for platform-specific code.
# GOTAGS is picked up by the main Makefile to set GOFLAGS and --build-arg for Docker.
GOTAGS = distro
export GOFLAGS += -tags=$(GOTAGS)

# Align image names with ODH registry conventions so that `make docker-build-*`
# produces images that match CI expectations without re-tagging.
AGENT_IMG = kserve-agent
ROUTER_IMG = kserve-router
STORAGE_INIT_IMG = kserve-storage-initializer

.PHONY: deploy-dev-llm-ocp deploy-ci uv-update-lockfiles chaos-validate

-include Makefile.ocp.mk

deploy-dev-llm-ocp:
	./test/scripts/openshift-ci/setup-llm.sh --deploy-kuadrant

deploy-ci: manifests
	kubectl apply --server-side=true --force-conflicts -k config/crd/full
	kubectl apply --server-side=true --force-conflicts -k config/crd/full/localmodel
	kubectl apply --server-side=true --force-conflicts -k config/crd/full/llmisvc
	kubectl wait --for=condition=established --timeout=60s crd/llminferenceserviceconfigs.serving.kserve.io
	kubectl apply --server-side=true -k config/overlays/test
	kubectl wait --for=condition=ready pod -l control-plane=kserve-controller-manager -n kserve --timeout=300s
	kubectl apply --server-side=true -k config/overlays/test/clusterresources

uv-update-lockfiles:
	bash -ec 'for value in $$(find . -name uv.lock -exec dirname {} \;); do (cd "$${value}" && echo "Updating $${value}/uv.lock" && uv update --lock); done'

manifests-distro: controller-gen
	@$(CONTROLLER_GEN) rbac:roleName=kserve-inferenceservice-distro-role \
		paths=./pkg/controller/v1beta1/inferenceservice/distro \
		output:rbac:artifacts:config=config/overlays/odh/rbac/inferenceservice
	@$(CONTROLLER_GEN) rbac:roleName=kserve-llmisvc-distro-role \
		paths=./pkg/controller/v1alpha2/llmisvc/distro \
		output:rbac:artifacts:config=config/overlays/odh/rbac/llmisvc
	@$(CONTROLLER_GEN) rbac:roleName=kserve-localmodel-distro-role \
		paths=./pkg/controller/v1alpha1/localmodel/distro \
		output:rbac:artifacts:config=config/overlays/odh-modelcache/rbac/localmodel
	@$(CONTROLLER_GEN) rbac:roleName=kserve-localmodelnode-distro-role \
		paths=./pkg/controller/v1alpha1/localmodelnode/distro \
		output:rbac:artifacts:config=config/overlays/odh-modelcache/rbac/localmodelnode
	@$(CONTROLLER_GEN) rbac:roleName=kserve-tls-distro-role \
		paths=./pkg/tls/distro \
		output:rbac:artifacts:config=config/overlays/odh/rbac/tls

## operator-chaos tooling
OPERATOR_CHAOS = $(LOCALBIN)/operator-chaos
OPERATOR_CHAOS_VERSION ?= v0.0.0-20260616171738-edb1c045f677

.PHONY: operator-chaos
operator-chaos: $(OPERATOR_CHAOS)
$(OPERATOR_CHAOS): $(LOCALBIN)
	test -s $(LOCALBIN)/operator-chaos || GOTOOLCHAIN=auto GOBIN=$(LOCALBIN) go install github.com/opendatahub-io/operator-chaos/cmd/operator-chaos@$(OPERATOR_CHAOS_VERSION)

chaos-validate: operator-chaos ## Validate chaos knowledge model and experiments.
	$(OPERATOR_CHAOS) validate --knowledge chaos/knowledge/kserve.yaml
	$(OPERATOR_CHAOS) preflight --knowledge chaos/knowledge/kserve.yaml --local
	@status=0; \
	for f in chaos/experiments/*.yaml; do \
		echo "--- $$f ---"; \
		if ! $(OPERATOR_CHAOS) validate "$$f"; then \
			status=1; \
		fi; \
	done; \
	exit $$status

## ODH overlay verification
# Verify the opendatahub.io/runtime-version annotation stamping on the
# accelerator LLMInferenceServiceConfig presets rendered by config/overlays/odh.
.PHONY: verify-odh-runtime-version
verify-odh-runtime-version: kustomize yq
	@KUSTOMIZE=$(KUSTOMIZE) YQ=$(YQ) bash hack/verify-odh-runtime-version.sh

# Chain into upstream's precommit (and thus `make check` in precommit-check CI)
# from this midstream-only file, so the upstream-owned precommit line in the
# main Makefile stays untouched across upstream syncs.
precommit: verify-odh-runtime-version

-include Makefile.kserve-module.mk

