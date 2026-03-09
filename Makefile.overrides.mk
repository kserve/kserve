# Midstream-only Make targets for opendatahub-io/kserve.
# Loaded via `-include Makefile.overrides.mk` in the main Makefile.
# This file does not exist on upstream kserve/kserve.

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
