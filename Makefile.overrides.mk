# Midstream-only Make targets for opendatahub-io/kserve.
# Loaded via `-include Makefile.overrides.mk` in the main Makefile.
# This file does not exist on upstream kserve/kserve.

# Enable distro build tag for platform-specific code.
# GOTAGS is picked up by the main Makefile to set GOFLAGS and --build-arg for Docker.
GOTAGS = distro
export GOFLAGS += -tags=$(GOTAGS)

.PHONY: deploy-dev-llm deploy-dev-llm-ocp deploy-ci uv-update-lockfiles

deploy-dev-llm:
	./hack/deploy_dev_llm.sh

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
	@$(CONTROLLER_GEN) rbac:roleName=kserve-llmisvc-distro-role \
		paths=./pkg/controller/v1alpha2/llmisvc/distro \
		output:rbac:artifacts:config=config/overlays/odh/rbac/llmisvc
	@$(CONTROLLER_GEN) rbac:roleName=kserve-localmodel-distro-role \
		paths=./pkg/controller/v1alpha1/localmodel/distro \
		output:rbac:artifacts:config=config/overlays/odh-modelcache/rbac/localmodel
	@$(CONTROLLER_GEN) rbac:roleName=kserve-localmodelnode-distro-role \
		paths=./pkg/controller/v1alpha1/localmodelnode/distro \
		output:rbac:artifacts:config=config/overlays/odh-modelcache/rbac/localmodelnode
