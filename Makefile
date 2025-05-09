# The Go and Python based tools are defined in Makefile.tools.mk.
include Makefile.tools.mk

# Base Image URL
BASE_IMG ?= python:3.11-slim-bookworm
PMML_BASE_IMG ?= openjdk:21-slim-bookworm

# Image URL to use all building/pushing image targets
IMG ?= kserve-controller:latest
AGENT_IMG ?= agent:latest
ROUTER_IMG ?= router:latest
SKLEARN_IMG ?= sklearnserver
XGB_IMG ?= xgbserver
LGB_IMG ?= lgbserver
PMML_IMG ?= pmmlserver
PADDLE_IMG ?= paddleserver
CUSTOM_MODEL_IMG ?= custom-model
CUSTOM_MODEL_GRPC_IMG ?= custom-model-grpc
CUSTOM_TRANSFORMER_IMG ?= image-transformer
CUSTOM_TRANSFORMER_GRPC_IMG ?= custom-image-transformer-grpc
HUGGINGFACE_SERVER_IMG ?= huggingfaceserver
HUGGINGFACE_SERVER_CPU_IMG ?= huggingfaceserver-cpu
AIF_IMG ?= aiffairness
ART_IMG ?= art-explainer
STORAGE_INIT_IMG ?= storage-initializer
QPEXT_IMG ?= qpext:latest
SUCCESS_200_ISVC_IMG ?= success-200-isvc
ERROR_404_ISVC_IMG ?= error-404-isvc

CRD_OPTIONS ?= "crd:maxDescLen=0"
KSERVE_ENABLE_SELF_SIGNED_CA ?= false

# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.29

ENGINE ?= docker
# Empty string for local build when using podman, it allows to build different architectures
# to use do: ENGINE=podman ARCH="--arch x86_64" make docker-build-something
ARCH ?=

# CPU/Memory limits for controller-manager
KSERVE_CONTROLLER_CPU_LIMIT ?= 100m
KSERVE_CONTROLLER_MEMORY_LIMIT ?= 300Mi
$(shell perl -pi -e 's/cpu:.*/cpu: $(KSERVE_CONTROLLER_CPU_LIMIT)/' config/default/manager_resources_patch.yaml)
$(shell perl -pi -e 's/memory:.*/memory: $(KSERVE_CONTROLLER_MEMORY_LIMIT)/' config/default/manager_resources_patch.yaml)

export GOFLAGS=-mod=mod

all: test manager agent router

# Run go fmt against code
fmt:
	go fmt ./pkg/... ./cmd/... && cd qpext && go fmt ./...

py-fmt: $(BLACK_FMT)
	$(BLACK_FMT) --config python/pyproject.toml .

# Run go vet against code
vet:
	go vet ./pkg/... ./cmd/... && cd qpext && go vet ./...

tidy:
	go mod tidy
	cd qpext && go mod tidy

go-lint: golangci-lint
	@$(GOLANGCI_LINT) run --fix

py-lint: $(FLAKE8_LINT)
	$(FLAKE8_LINT) --config=.flake8 .

# Generate manifests e.g. CRD, RBAC etc.
manifests: controller-gen yq
	@$(CONTROLLER_GEN) $(CRD_OPTIONS) paths=./pkg/apis/serving/... output:crd:dir=config/crd/full
	@$(CONTROLLER_GEN) rbac:roleName=kserve-manager-role paths={./pkg/controller/v1alpha1/inferencegraph,./pkg/controller/v1alpha1/trainedmodel,./pkg/controller/v1beta1/...} output:rbac:artifacts:config=config/rbac
	@$(CONTROLLER_GEN) rbac:roleName=kserve-localmodel-manager-role paths=./pkg/controller/v1alpha1/localmodel output:rbac:artifacts:config=config/rbac/localmodel
	@$(CONTROLLER_GEN) rbac:roleName=kserve-localmodelnode-agent-role paths=./pkg/controller/v1alpha1/localmodelnode output:rbac:artifacts:config=config/rbac/localmodelnode
	# Copy the cluster role to the helm chart
	cp config/rbac/auth_proxy_role.yaml charts/kserve-resources/templates/clusterrole.yaml
	cat config/rbac/role.yaml >> charts/kserve-resources/templates/clusterrole.yaml
	# Copy the local model role with Helm chart while keeping the Helm template condition
	echo '{{- if .Values.kserve.localmodel.enabled }}' > charts/kserve-resources/templates/localmodel/role.yaml
	cat config/rbac/localmodel/role.yaml >> charts/kserve-resources/templates/localmodel/role.yaml
	echo '{{- end }}' >> charts/kserve-resources/templates/localmodel/role.yaml
	# Copy the local model node role with Helm chart while keeping the Helm template condition
	echo '{{- if .Values.kserve.localmodel.enabled }}'> charts/kserve-resources/templates/localmodelnode/role.yaml
	cat config/rbac/localmodelnode/role.yaml >> charts/kserve-resources/templates/localmodelnode/role.yaml
	echo '{{- end }}' >> charts/kserve-resources/templates/localmodelnode/role.yaml
	
	@$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths=./pkg/apis/serving/v1alpha1
	@$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths=./pkg/apis/serving/v1beta1

	#remove the required property on framework as name field needs to be optional
	@$(YQ) 'del(.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.*.properties.*.required)' -i config/crd/full/serving.kserve.io_inferenceservices.yaml
	#remove ephemeralContainers properties for compress crd size https://github.com/kubeflow/kfserving/pull/1141#issuecomment-714170602
	@$(YQ) 'del(.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.*.properties.ephemeralContainers)' -i config/crd/full/serving.kserve.io_inferenceservices.yaml
	#knative does not allow setting port on liveness or readiness probe
	@$(YQ) 'del(.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.*.properties.*.properties.readinessProbe.properties.httpGet.required)' -i config/crd/full/serving.kserve.io_inferenceservices.yaml
	@$(YQ) 'del(.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.*.properties.*.properties.livenessProbe.properties.httpGet.required)' -i config/crd/full/serving.kserve.io_inferenceservices.yaml
	@$(YQ) 'del(.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.*.properties.*.properties.readinessProbe.properties.tcpSocket.required)' -i config/crd/full/serving.kserve.io_inferenceservices.yaml
	@$(YQ) 'del(.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.*.properties.*.properties.livenessProbe.properties.tcpSocket.required)' -i config/crd/full/serving.kserve.io_inferenceservices.yaml
	@$(YQ) 'del(.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.*.properties.containers.items.properties.livenessProbe.properties.httpGet.required)' -i config/crd/full/serving.kserve.io_inferenceservices.yaml
	@$(YQ) 'del(.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.*.properties.containers.items.properties.readinessProbe.properties.httpGet.required)' -i config/crd/full/serving.kserve.io_inferenceservices.yaml
	#With v1 and newer kubernetes protocol requires default
	@$(YQ) '.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties | .. | select(has("protocol")) | path' config/crd/full/serving.kserve.io_inferenceservices.yaml -o j | jq -r '. | map(select(numbers)="["+tostring+"]") | join(".")' | awk '{print "."$$0".protocol.default"}' | xargs -n1 -I{} $(YQ) '{} = "TCP"' -i config/crd/full/serving.kserve.io_inferenceservices.yaml
	@$(YQ) '.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties | .. | select(has("protocol")) | path' config/crd/full/serving.kserve.io_clusterservingruntimes.yaml -o j | jq -r '. | map(select(numbers)="["+tostring+"]") | join(".")' | awk '{print "."$$0".protocol.default"}' | xargs -n1 -I{} $(YQ) '{} = "TCP"' -i config/crd/full/serving.kserve.io_clusterservingruntimes.yaml
	@$(YQ) '.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties | .. | select(has("protocol")) | path' config/crd/full/serving.kserve.io_servingruntimes.yaml -o j | jq -r '. | map(select(numbers)="["+tostring+"]") | join(".")' | awk '{print "."$$0".protocol.default"}' | xargs -n1 -I{} $(YQ) '{} = "TCP"' -i config/crd/full/serving.kserve.io_servingruntimes.yaml
	
	# TODO: Commenting out the following as it produces differences in verify codegen during release process
	# Copy the crds to the helm chart
	# cp config/crd/full/* charts/kserve-crd/templates
	# rm charts/kserve-crd/templates/kustomization.yaml
	# Generate minimal crd
	./hack/minimal-crdgen.sh
	kubectl kustomize config/crd/full > test/crds/serving.kserve.io_inferenceservices.yaml
	# Copy the minimal crd to the helm chart
	cp config/crd/minimal/* charts/kserve-crd-minimal/templates/
	rm charts/kserve-crd-minimal/templates/kustomization.yaml

# Generate code
generate: controller-gen helm-docs
	hack/update-codegen.sh
	hack/update-openapigen.sh
	hack/python-sdk/client-gen.sh
	$(HELM_DOCS) --chart-search-root=charts --output-file=README.md

# Update poetry.lock files
poetry-lock: $(POETRY)
# Update the kserve package first as other packages depends on it.
	cd ./python && \
	cd kserve && $(POETRY) lock --no-update && cd .. && \
	for file in $$(find . -type f -name "pyproject.toml" -not -path "./pyproject.toml" -not -path "*.venv/*"); do \
		folder=$$(dirname "$$file"); \
		echo "moving into folder $$folder"; \
		case "$$folder" in \
			*plugin*|plugin|kserve) \
				echo -e "\033[33mSkipping folder $$folder\033[0m" ;; \
			*) \
				cd "$$folder" && $(POETRY) lock --no-update && cd - > /dev/null ;; \
		esac; \
	done

# This runs all necessary steps to prepare for a commit.
precommit: vet tidy go-lint py-fmt py-lint generate manifests poetry-lock

# This is used by CI to ensure that the precommit checks are met.
check: precommit
	@if [ ! -z "`git status -s`" ]; then \
		echo "The following differences will fail CI until committed:"; \
		git diff --exit-code; \
		echo "Please ensure that you have run 'make precommit' and committed the changes."; \
		exit 1; \
	fi

# This clears all the installed binaries.
#
# Whenever you run into issues with the target like `precommit` or `test`, try running this target.
.PHONY: clean
clean:
	rm -rf $(LOCALBIN)

# Run tests
test: fmt vet manifests envtest test-qpext
	KUBEBUILDER_ASSETS="$$($(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" go test $$(go list ./pkg/...) ./cmd/... -coverprofile coverage.out -coverpkg ./pkg/... ./cmd...

test-qpext:
	cd qpext && go test -v ./... -cover

# Build manager binary
manager: generate fmt vet go-lint
	go build -o bin/manager ./cmd/manager

# Build agent binary
agent: fmt vet
	go build -o bin/agent ./cmd/agent

# Build router binary
router: fmt vet
	go build -o bin/router ./cmd/router

# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate fmt vet go-lint
	go run ./cmd/manager/main.go

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: manifests
	# Remove the certmanager certificate if KSERVE_ENABLE_SELF_SIGNED_CA is not false
	cd config/default && if [ ${KSERVE_ENABLE_SELF_SIGNED_CA} != false ]; then \
	echo > ../certmanager/certificate.yaml; \
	else git checkout HEAD -- ../certmanager/certificate.yaml; fi;
	kubectl apply --server-side=true -k config/default
	if [ ${KSERVE_ENABLE_SELF_SIGNED_CA} != false ]; then ./hack/self-signed-ca.sh; fi;
	kubectl wait --for=condition=ready pod -l control-plane=kserve-controller-manager -n kserve --timeout=300s
	kubectl apply  --server-side=true  -k config/clusterresources
	git checkout HEAD -- config/certmanager/certificate.yaml


deploy-dev: manifests
	./hack/image_patch_dev.sh development
	# Remove the certmanager certificate if KSERVE_ENABLE_SELF_SIGNED_CA is not false
	cd config/default && if [ ${KSERVE_ENABLE_SELF_SIGNED_CA} != false ]; then \
	echo > ../certmanager/certificate.yaml; \
	else git checkout HEAD -- ../certmanager/certificate.yaml; fi;
	kubectl apply --server-side=true --force-conflicts -k config/overlays/development
	if [ ${KSERVE_ENABLE_SELF_SIGNED_CA} != false ]; then ./hack/self-signed-ca.sh; fi;
	# TODO: Add runtimes as part of default deployment
	kubectl wait --for=condition=ready pod -l control-plane=kserve-controller-manager -n kserve --timeout=300s
	kubectl apply --server-side=true --force-conflicts -k config/clusterresources
	git checkout HEAD -- config/certmanager/certificate.yaml

deploy-dev-sklearn: docker-push-sklearn
	./hack/serving_runtime_image_patch.sh "kserve-sklearnserver.yaml" "${KO_DOCKER_REPO}/${SKLEARN_IMG}"

deploy-dev-xgb: docker-push-xgb
	./hack/serving_runtime_image_patch.sh "kserve-xgbserver.yaml" "${KO_DOCKER_REPO}/${XGB_IMG}"

deploy-dev-lgb: docker-push-lgb
	./hack/serving_runtime_image_patch.sh "kserve-lgbserver.yaml" "${KO_DOCKER_REPO}/${LGB_IMG}"

deploy-dev-pmml : docker-push-pmml
	./hack/serving_runtime_image_patch.sh "kserve-pmmlserver.yaml" "${KO_DOCKER_REPO}/${PMML_IMG}"

deploy-dev-paddle: docker-push-paddle
	./hack/serving_runtime_image_patch.sh "kserve-paddleserver.yaml" "${KO_DOCKER_REPO}/${PADDLE_IMG}"

deploy-dev-huggingface: docker-push-huggingface
	./hack/serving_runtime_image_patch.sh "kserve-huggingfaceserver.yaml" "${KO_DOCKER_REPO}/${HUGGINGFACE_SERVER_IMG}"

deploy-dev-storageInitializer: docker-push-storageInitializer
	./hack/storageInitializer_patch_dev.sh ${KO_DOCKER_REPO}/${STORAGE_INIT_IMG}
	kubectl apply --server-side=true -k config/overlays/dev-image-config

deploy-ci: manifests
	kubectl apply --server-side=true -k config/overlays/test
	# TODO: Add runtimes as part of default deployment
	kubectl wait --for=condition=ready pod -l control-plane=kserve-controller-manager -n kserve --timeout=300s
	kubectl apply --server-side=true -k config/overlays/test/clusterresources

deploy-helm: manifests
	helm install kserve-crd charts/kserve-crd/ --wait --timeout 180s
	helm install kserve charts/kserve-resources/ --wait --timeout 180s -n kserve --create-namespace

undeploy:
	kubectl delete -k config/default

undeploy-dev:
	kubectl delete -k config/overlays/development

bump-version:
	@echo "bumping version numbers for this release"
	@hack/prepare-for-release.sh $(PRIOR_VERSION) $(NEW_VERSION)

# Build the docker image
docker-build:
	${ENGINE} buildx build ${ARCH} . -t ${IMG}
	@echo "updating kustomize image patch file for manager resource"

	# Use perl instead of sed to avoid OSX/Linux compatibility issue:
	# https://stackoverflow.com/questions/34533893/sed-command-creating-unwanted-duplicates-of-file-with-e-extension
	perl -pi -e 's@image: .*@image: '"${IMG}"'@' ./config/default/manager_image_patch.yaml

# Push the docker image
docker-push:
	docker push ${IMG}

docker-build-agent:
	${ENGINE} buildx build ${ARCH} -f agent.Dockerfile . -t ${KO_DOCKER_REPO}/${AGENT_IMG}

docker-build-router:
	${ENGINE} buildx build ${ARCH} -f router.Dockerfile . -t ${KO_DOCKER_REPO}/${ROUTER_IMG}

docker-push-agent:
	${ENGINE} push ${KO_DOCKER_REPO}/${AGENT_IMG}

docker-push-router:
	${ENGINE} push ${KO_DOCKER_REPO}/${ROUTER_IMG}

docker-build-sklearn:
	cd python && ${ENGINE} buildx build ${ARCH} --build-arg BASE_IMAGE=${BASE_IMG} -t ${KO_DOCKER_REPO}/${SKLEARN_IMG} -f sklearn.Dockerfile .

docker-push-sklearn: docker-build-sklearn
	${ENGINE} push ${KO_DOCKER_REPO}/${SKLEARN_IMG}

docker-build-xgb:
	cd python && ${ENGINE} buildx build ${ARCH} --build-arg BASE_IMAGE=${BASE_IMG} -t ${KO_DOCKER_REPO}/${XGB_IMG} -f xgb.Dockerfile .

docker-push-xgb: docker-build-xgb
	${ENGINE} push ${KO_DOCKER_REPO}/${XGB_IMG}

docker-build-lgb:
	cd python && ${ENGINE} buildx build ${ARCH} --build-arg BASE_IMAGE=${BASE_IMG} -t ${KO_DOCKER_REPO}/${LGB_IMG} -f lgb.Dockerfile .

docker-push-lgb: docker-build-lgb
	${ENGINE} push ${KO_DOCKER_REPO}/${LGB_IMG}

docker-build-pmml:
	cd python && ${ENGINE} buildx build ${ARCH} --build-arg BASE_IMAGE=${PMML_BASE_IMG} -t ${KO_DOCKER_REPO}/${PMML_IMG} -f pmml.Dockerfile .

docker-push-pmml: docker-build-pmml
	${ENGINE} push ${KO_DOCKER_REPO}/${PMML_IMG}

docker-build-paddle:
	cd python && ${ENGINE} buildx build ${ARCH} --build-arg BASE_IMAGE=${BASE_IMG} -t ${KO_DOCKER_REPO}/${PADDLE_IMG} -f paddle.Dockerfile .

docker-push-paddle: docker-build-paddle
	${ENGINE} push ${KO_DOCKER_REPO}/${PADDLE_IMG}

docker-build-custom-model:
	cd python && ${ENGINE} buildx build ${ARCH} -t ${KO_DOCKER_REPO}/${CUSTOM_MODEL_IMG} -f custom_model.Dockerfile .

docker-push-custom-model: docker-build-custom-model
	docker push ${KO_DOCKER_REPO}/${CUSTOM_MODEL_IMG}

docker-build-custom-model-grpc:
	cd python && ${ENGINE} buildx build ${ARCH} -t ${KO_DOCKER_REPO}/${CUSTOM_MODEL_GRPC_IMG} -f custom_model_grpc.Dockerfile .

docker-push-custom-model-grpc: docker-build-custom-model-grpc
	${ENGINE} push ${KO_DOCKER_REPO}/${CUSTOM_MODEL_GRPC_IMG}

docker-build-custom-transformer:
	cd python && ${ENGINE} buildx build ${ARCH} -t ${KO_DOCKER_REPO}/${CUSTOM_TRANSFORMER_IMG} -f custom_transformer.Dockerfile .

docker-push-custom-transformer: docker-build-custom-transformer
	${ENGINE} push ${KO_DOCKER_REPO}/${CUSTOM_TRANSFORMER_IMG}

docker-build-custom-transformer-grpc:
	cd python && ${ENGINE} buildx build ${ARCH} -t ${KO_DOCKER_REPO}/${CUSTOM_TRANSFORMER_GRPC_IMG} -f custom_transformer_grpc.Dockerfile .

docker-push-custom-transformer-grpc: docker-build-custom-transformer-grpc
	${ENGINE} push ${KO_DOCKER_REPO}/${CUSTOM_TRANSFORMER_GRPC_IMG}

docker-build-aif:
	cd python && ${ENGINE} buildx build ${ARCH} -t ${KO_DOCKER_REPO}/${AIF_IMG} -f aiffairness.Dockerfile .

docker-push-aif: docker-build-aif
	${ENGINE} push ${KO_DOCKER_REPO}/${AIF_IMG}

docker-build-art:
	cd python && ${ENGINE} buildx build ${ARCH} -t ${KO_DOCKER_REPO}/${ART_IMG} -f artexplainer.Dockerfile .

docker-push-art: docker-build-art
	${ENGINE} push ${KO_DOCKER_REPO}/${ART_IMG}

docker-build-storageInitializer:
	cd python && ${ENGINE} buildx build ${ARCH} --build-arg BASE_IMAGE=${BASE_IMG} -t ${KO_DOCKER_REPO}/${STORAGE_INIT_IMG} -f storage-initializer.Dockerfile .

docker-push-storageInitializer: docker-build-storageInitializer
	${ENGINE} push ${KO_DOCKER_REPO}/${STORAGE_INIT_IMG}

docker-build-qpext:
	${ENGINE} buildx build ${ARCH} -t ${KO_DOCKER_REPO}/${QPEXT_IMG} -f qpext/qpext.Dockerfile .

docker-build-push-qpext: docker-build-qpext
	${ENGINE} push ${KO_DOCKER_REPO}/${QPEXT_IMG}

deploy-dev-qpext: docker-build-push-qpext
	kubectl patch cm config-deployment -n knative-serving --type merge --patch '{"data": {"queue-sidecar-image": "${KO_DOCKER_REPO}/${QPEXT_IMG}"}}'

docker-build-success-200-isvc:
	cd python && ${ENGINE} buildx build ${ARCH} -t ${KO_DOCKER_REPO}/${SUCCESS_200_ISVC_IMG} -f success_200_isvc.Dockerfile .

docker-push-success-200-isvc: docker-build-success-200-isvc
	${ENGINE} push ${KO_DOCKER_REPO}/${SUCCESS_200_ISVC_IMG}

docker-build-error-node-404:
	cd python && ${ENGINE} buildx build ${ARCH} -t ${KO_DOCKER_REPO}/${ERROR_404_ISVC_IMG} -f error_404_isvc.Dockerfile .

docker-push-error-node-404: docker-build-error-node-404
	${ENGINE} push ${KO_DOCKER_REPO}/${ERROR_404_ISVC_IMG}

docker-build-huggingface:
	cd python && ${ENGINE} buildx build ${ARCH} -t ${KO_DOCKER_REPO}/${HUGGINGFACE_SERVER_IMG} -f huggingface_server.Dockerfile .

docker-push-huggingface: docker-build-huggingface
	${ENGINE} push ${KO_DOCKER_REPO}/${HUGGINGFACE_SERVER_IMG}

docker-build-huggingface-cpu:
	cd python && ${ENGINE} buildx build ${ARCH} -t ${KO_DOCKER_REPO}/${HUGGINGFACE_SERVER_CPU_IMG} -f huggingface_server_cpu.Dockerfile .

docker-push-huggingface-cpu: docker-build-huggingface-cpu
	${ENGINE} push ${KO_DOCKER_REPO}/${HUGGINGFACE_SERVER_CPU_IMG}

apidocs:
	${ENGINE} buildx build ${ARCH} -f docs/apis/Dockerfile --rm -t apidocs-gen . && \
	${ENGINE} run -it --rm -v $(CURDIR)/pkg/apis:/go/src/github.com/kserve/kserve/pkg/apis -v ${PWD}/docs/apis:/go/gen-crd-api-reference-docs/apidocs apidocs-gen

.PHONY: check-doc-links
check-doc-links:
	@python3 hack/verify-doc-links.py && echo "$@: OK"

poetry-update-lockfiles:
	bash -ec 'for value in $$(find . -name poetry.lock -exec dirname {} \;); do (cd "$${value}" && echo "Updating $${value}/poetry.lock" && poetry update --lock); done'
