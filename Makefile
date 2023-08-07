HAS_LINT := $(shell command -v golint;)

# Base Image URL
PYTHON_BASE_IMG ?= python:3.9-slim-bullseye
PMML_BASE_IMG ?= openjdk:11-slim

# Image URL to use all building/pushing image targets
IMG ?= kserve-controller:latest
AGENT_IMG ?= agent:latest
ROUTER_IMG ?= router:latest
BUILD_BASE_IMG ?= build-base-image:latest
PROD_BASE_IMG ?= prod-base-image:latest
SKLEARN_IMG ?= sklearnserver
XGB_IMG ?= xgbserver
LGB_IMG ?= lgbserver
PMML_IMG ?= pmmlserver
PADDLE_IMG ?= paddleserver
CUSTOM_MODEL_IMG ?= custom-model
CUSTOM_MODEL_GRPC_IMG ?= custom-model-grpc
CUSTOM_TRANSFORMER_IMG ?= image-transformer
CUSTOM_TRANSFORMER_GRPC_IMG ?= custom-image-transformer-grpc
ALIBI_IMG ?= alibi-explainer
AIF_IMG ?= aiffairness
ART_IMG ?= art-explainer
STORAGE_INIT_IMG ?= storage-initializer
QPEXT_IMG ?= qpext
CRD_OPTIONS ?= "crd:maxDescLen=0"
KSERVE_ENABLE_SELF_SIGNED_CA ?= false
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.26

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KUSTOMIZE ?= $(LOCALBIN)/kustomize
ENVTEST ?= $(LOCALBIN)/setup-envtest
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen

## Tool Versions
KUSTOMIZE_VERSION ?= v5.0.3
CONTROLLER_TOOLS_VERSION ?= v0.12.0

# CPU/Memory limits for controller-manager
KSERVE_CONTROLLER_CPU_LIMIT ?= 100m
KSERVE_CONTROLLER_MEMORY_LIMIT ?= 300Mi
$(shell perl -pi -e 's/cpu:.*/cpu: $(KSERVE_CONTROLLER_CPU_LIMIT)/' config/default/manager_resources_patch.yaml)
$(shell perl -pi -e 's/memory:.*/memory: $(KSERVE_CONTROLLER_MEMORY_LIMIT)/' config/default/manager_resources_patch.yaml)

all: test manager agent router

# Run tests
test: fmt vet manifests envtest
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" go test $$(go list ./pkg/...) ./cmd/... -coverprofile coverage.out -coverpkg ./pkg/... ./cmd...

# Build manager binary
manager: generate fmt vet lint
	go build -o bin/manager ./cmd/manager

# Build agent binary
agent: fmt vet
	go build -o bin/agent ./cmd/agent

# Build router binary
router: fmt vet
	go build -o bin/router ./cmd/router

# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate fmt vet lint
	go run ./cmd/manager/main.go

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: manifests
	# Remove the certmanager certificate if KSERVE_ENABLE_SELF_SIGNED_CA is not false
	cd config/default && if [ ${KSERVE_ENABLE_SELF_SIGNED_CA} != false ]; then \
	${KUSTOMIZE} edit remove resource certmanager/certificate.yaml; \
	else ${KUSTOMIZE} edit add resource certmanager/certificate.yaml; fi;
	${KUSTOMIZE} build config/default | kubectl apply -f -
	kubectl wait --for=condition=ready pod -l control-plane=kserve-controller-manager -n kserve --timeout=300s
	sleep 2
	${KUSTOMIZE} build config/runtimes | kubectl apply -f -
	if [ ${KSERVE_ENABLE_SELF_SIGNED_CA} != false ]; then ./hack/self-signed-ca.sh; fi;

deploy-dev: manifests
	./hack/image_patch_dev.sh development
	# Remove the certmanager certificate if KSERVE_ENABLE_SELF_SIGNED_CA is not false
	cd config/default && if [ ${KSERVE_ENABLE_SELF_SIGNED_CA} != false ]; then \
	${KUSTOMIZE} edit remove resource certmanager/certificate.yaml; \
	else ${KUSTOMIZE} edit add resource certmanager/certificate.yaml; fi;
	${KUSTOMIZE} build config/overlays/development | kubectl apply -f -
	# TODO: Add runtimes as part of default deployment
	kubectl wait --for=condition=ready pod -l control-plane=kserve-controller-manager -n kserve --timeout=300s
	sleep 2
	${KUSTOMIZE} build config/runtimes | kubectl apply -f -
	if [ ${KSERVE_ENABLE_SELF_SIGNED_CA} != false ]; then ./hack/self-signed-ca.sh; fi;

deploy-dev-sklearn: docker-push-sklearn kustomize
	./hack/serving_runtime_image_patch.sh "kserve-sklearnserver.yaml" "${KO_DOCKER_REPO}/${SKLEARN_IMG}"

deploy-dev-xgb: docker-push-xgb kustomize
	./hack/serving_runtime_image_patch.sh "kserve-xgbserver.yaml" "${KO_DOCKER_REPO}/${XGB_IMG}"

deploy-dev-lgb: docker-push-lgb kustomize
	./hack/serving_runtime_image_patch.sh "kserve-lgbserver.yaml" "${KO_DOCKER_REPO}/${LGB_IMG}"

deploy-dev-pmml : docker-push-pmml
	./hack/serving_runtime_image_patch.sh "kserve-pmmlserver.yaml" "${KO_DOCKER_REPO}/${PMML_IMG}"

deploy-dev-paddle: docker-push-paddle
	./hack/serving_runtime_image_patch.sh "kserve-paddleserver.yaml" "${KO_DOCKER_REPO}/${PADDLE_IMG}"

deploy-dev-alibi: docker-push-alibi kustomize
	./hack/alibi_patch_dev.sh ${KO_DOCKER_REPO}/${ALIBI_IMG}
	${KUSTOMIZE} build config/overlays/dev-image-config | kubectl apply -f -

deploy-dev-storageInitializer: docker-push-storageInitializer kustomize
	./hack/storageInitializer_patch_dev.sh ${KO_DOCKER_REPO}/${STORAGE_INIT_IMG}
	${KUSTOMIZE} build config/overlays/dev-image-config | kubectl apply -f -

deploy-ci: manifests
	kubectl apply -k config/overlays/test
	# TODO: Add runtimes as part of default deployment
	kubectl wait --for=condition=ready pod -l control-plane=kserve-controller-manager -n kserve --timeout=300s
	kubectl apply -k config/overlays/test/runtimes

deploy-helm: manifests
	helm install kserve-crd charts/kserve-crd/ --wait --timeout 180s
	helm install kserve charts/kserve-resources/ --wait --timeout 180s

undeploy: kustomize
	${KUSTOMIZE} build config/default | kubectl delete -f -

undeploy-dev: kustomize
	${KUSTOMIZE} build config/overlays/development | kubectl delete -f -

# Generate manifests e.g. CRD, RBAC etc.
manifests: controller-gen kustomize
	$(CONTROLLER_GEN) $(CRD_OPTIONS) paths=./pkg/apis/serving/... output:crd:dir=config/crd
	$(CONTROLLER_GEN) rbac:roleName=kserve-manager-role paths=./pkg/controller/... output:rbac:artifacts:config=config/rbac
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths=./pkg/apis/serving/v1alpha1
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths=./pkg/apis/serving/v1beta1

	#TODO Remove this until new controller-tools is released
	perl -pi -e 's/storedVersions: null/storedVersions: []/g' config/crd/serving.kserve.io_inferenceservices.yaml
	perl -pi -e 's/conditions: null/conditions: []/g' config/crd/serving.kserve.io_inferenceservices.yaml
	perl -pi -e 's/Any/string/g' config/crd/serving.kserve.io_inferenceservices.yaml
	perl -pi -e 's/storedVersions: null/storedVersions: []/g' config/crd/serving.kserve.io_trainedmodels.yaml
	perl -pi -e 's/conditions: null/conditions: []/g' config/crd/serving.kserve.io_trainedmodels.yaml
	perl -pi -e 's/Any/string/g' config/crd/serving.kserve.io_trainedmodels.yaml
	perl -pi -e 's/storedVersions: null/storedVersions: []/g' config/crd/serving.kserve.io_inferencegraphs.yaml
	perl -pi -e 's/conditions: null/conditions: []/g' config/crd/serving.kserve.io_inferencegraphs.yaml
	perl -pi -e 's/Any/string/g' config/crd/serving.kserve.io_inferencegraphs.yaml
	#remove the required property on framework as name field needs to be optional
	yq 'del(.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.*.properties.*.required)' -i config/crd/serving.kserve.io_inferenceservices.yaml
	#remove ephemeralContainers properties for compress crd size https://github.com/kubeflow/kfserving/pull/1141#issuecomment-714170602
	yq 'del(.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.*.properties.ephemeralContainers)' -i config/crd/serving.kserve.io_inferenceservices.yaml 
	#knative does not allow setting port on liveness or readiness probe
	yq 'del(.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.*.properties.*.properties.readinessProbe.properties.httpGet.required)' -i config/crd/serving.kserve.io_inferenceservices.yaml 
	yq 'del(.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.*.properties.*.properties.livenessProbe.properties.httpGet.required)' -i config/crd/serving.kserve.io_inferenceservices.yaml 
	yq 'del(.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.*.properties.*.properties.readinessProbe.properties.tcpSocket.required)' -i config/crd/serving.kserve.io_inferenceservices.yaml 
	yq 'del(.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.*.properties.*.properties.livenessProbe.properties.tcpSocket.required)' -i config/crd/serving.kserve.io_inferenceservices.yaml 
	yq 'del(.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.*.properties.containers.items.properties.livenessProbe.properties.httpGet.required)' -i config/crd/serving.kserve.io_inferenceservices.yaml 
	yq 'del(.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.*.properties.containers.items.properties.readinessProbe.properties.httpGet.required)' -i config/crd/serving.kserve.io_inferenceservices.yaml 
	#With v1 and newer kubernetes protocol requires default
	yq '.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties | .. | select(has("protocol")) | path' config/crd/serving.kserve.io_inferenceservices.yaml -o j | jq -r '. | map(select(numbers)="["+tostring+"]") | join(".")' | awk '{print "."$$0".protocol.default"}' | xargs -n1 -I{} yq '{} = "TCP"' -i config/crd/serving.kserve.io_inferenceservices.yaml
	yq '.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties | .. | select(has("protocol")) | path' config/crd/serving.kserve.io_clusterservingruntimes.yaml -o j | jq -r '. | map(select(numbers)="["+tostring+"]") | join(".")' | awk '{print "."$$0".protocol.default"}' | xargs -n1 -I{} yq '{} = "TCP"' -i config/crd/serving.kserve.io_clusterservingruntimes.yaml
	yq '.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties | .. | select(has("protocol")) | path' config/crd/serving.kserve.io_servingruntimes.yaml -o j | jq -r '. | map(select(numbers)="["+tostring+"]") | join(".")' | awk '{print "."$$0".protocol.default"}' | xargs -n1 -I{} yq '{} = "TCP"' -i config/crd/serving.kserve.io_servingruntimes.yaml
	${KUSTOMIZE} build config/crd > test/crds/serving.kserve.io_inferenceservices.yaml

# Run go fmt against code
fmt:
	go fmt ./pkg/... ./cmd/...

# Run go vet against code
vet:
	go vet ./pkg/... ./cmd/...

lint:
ifndef HAS_LINT
	go get -u golang.org/x/lint/golint
	echo "installing golint"
endif
	hack/verify-golint.sh

# Generate code
generate: controller-gen
	go env -w GOFLAGS=-mod=mod
	hack/update-codegen.sh
	hack/update-openapigen.sh
	hack/python-sdk/client-gen.sh

# Build the docker image
docker-build: test
	docker buildx build . -t ${IMG}
	@echo "updating kustomize image patch file for manager resource"

	# Use perl instead of sed to avoid OSX/Linux compatibility issue:
	# https://stackoverflow.com/questions/34533893/sed-command-creating-unwanted-duplicates-of-file-with-e-extension
	perl -pi -e 's@image: .*@image: '"${IMG}"'@' ./config/default/manager_image_patch.yaml

# Push the docker image
docker-push:
	docker push ${IMG}

docker-build-agent:
	docker buildx build -f agent.Dockerfile . -t ${KO_DOCKER_REPO}/${AGENT_IMG}

docker-build-router:
	docker buildx build -f router.Dockerfile . -t ${KO_DOCKER_REPO}/${ROUTER_IMG}

docker-push-agent:
	docker push ${KO_DOCKER_REPO}/${AGENT_IMG}

docker-push-router:
	docker push ${KO_DOCKER_REPO}/${ROUTER_IMG}

docker-build-build-baseimage:
	cd python && docker buildx build --build-arg BASE_IMAGE=${PYTHON_BASE_IMG} -t ${BUILD_BASE_IMG} -f build_base_image.Dockerfile .

docker-build-prod-baseimage:
	cd python && docker buildx build --build-arg BASE_IMAGE=${PYTHON_BASE_IMG} -t ${PROD_BASE_IMG} -f prod_base_image.Dockerfile .

docker-build-sklearn: docker-build-build-baseimage docker-build-prod-baseimage
	cd python && docker buildx build -t ${KO_DOCKER_REPO}/${SKLEARN_IMG} -f sklearn.Dockerfile .

docker-push-sklearn: docker-build-sklearn
	docker push ${KO_DOCKER_REPO}/${SKLEARN_IMG}

docker-build-xgb: docker-build-build-baseimage docker-build-prod-baseimage
	cd python && docker buildx build -t ${KO_DOCKER_REPO}/${XGB_IMG} -f xgb.Dockerfile .

docker-push-xgb: docker-build-xgb
	docker push ${KO_DOCKER_REPO}/${XGB_IMG}

docker-build-lgb: docker-build-build-baseimage docker-build-prod-baseimage
	cd python && docker buildx build -t ${KO_DOCKER_REPO}/${LGB_IMG} -f lgb.Dockerfile .

docker-push-lgb: docker-build-lgb
	docker push ${KO_DOCKER_REPO}/${LGB_IMG}

docker-build-pmml:
	cd python && docker buildx build --build-arg BASE_IMAGE=${PMML_BASE_IMG} -t ${KO_DOCKER_REPO}/${PMML_IMG} -f pmml.Dockerfile .

docker-push-pmml: docker-build-pmml
	docker push ${KO_DOCKER_REPO}/${PMML_IMG}

docker-build-paddle: docker-build-build-baseimage docker-build-prod-baseimage
	cd python && docker buildx build -t ${KO_DOCKER_REPO}/${PADDLE_IMG} -f paddle.Dockerfile .

docker-push-paddle: docker-build-paddle
	docker push ${KO_DOCKER_REPO}/${PADDLE_IMG}

docker-build-custom-model: docker-build-build-baseimage docker-build-prod-baseimage
	cd python && docker buildx build -t ${KO_DOCKER_REPO}/${CUSTOM_MODEL_IMG} -f custom_model.Dockerfile .

docker-push-custom-model: docker-build-custom-model
	docker push ${KO_DOCKER_REPO}/${CUSTOM_MODEL_IMG}

docker-build-custom-model-grpc: docker-build-build-baseimage docker-build-prod-baseimage
	cd python && docker buildx build -t ${KO_DOCKER_REPO}/${CUSTOM_MODEL_GRPC_IMG} -f custom_model_grpc.Dockerfile .

docker-push-custom-model-grpc: docker-build-custom-model-grpc
	docker push ${KO_DOCKER_REPO}/${CUSTOM_MODEL_GRPC_IMG}

docker-build-custom-transformer: docker-build-build-baseimage docker-build-prod-baseimage
	cd python && docker buildx build -t ${KO_DOCKER_REPO}/${CUSTOM_TRANSFORMER_IMG} -f custom_transformer.Dockerfile .

docker-push-custom-transformer: docker-build-custom-transformer
	docker push ${KO_DOCKER_REPO}/${CUSTOM_TRANSFORMER_IMG}

docker-build-custom-transformer-grpc: docker-build-build-baseimage docker-build-prod-baseimage
	cd python && docker buildx build -t ${KO_DOCKER_REPO}/${CUSTOM_TRANSFORMER_GRPC_IMG} -f custom_transformer_grpc.Dockerfile .

docker-push-custom-transformer-grpc: docker-build-custom-transformer-grpc
	docker push ${KO_DOCKER_REPO}/${CUSTOM_TRANSFORMER_GRPC_IMG}

docker-build-alibi: docker-build-build-baseimage docker-build-prod-baseimage
	cd python && docker buildx build -t ${KO_DOCKER_REPO}/${ALIBI_IMG} -f alibiexplainer.Dockerfile .

docker-push-alibi: docker-build-alibi
	docker push ${KO_DOCKER_REPO}/${ALIBI_IMG}

docker-build-aif: docker-build-build-baseimage docker-build-prod-baseimage
	cd python && docker buildx build -t ${KO_DOCKER_REPO}/${AIF_IMG} -f aiffairness.Dockerfile .

docker-push-aif: docker-build-aif
	docker push ${KO_DOCKER_REPO}/${AIF_IMG}

docker-build-art: docker-build-build-baseimage docker-build-prod-baseimage
	cd python && docker buildx build -t ${KO_DOCKER_REPO}/${ART_IMG} -f artexplainer.Dockerfile .

docker-push-art: docker-build-art
	docker push ${KO_DOCKER_REPO}/${ART_IMG}

docker-build-storageInitializer: docker-build-build-baseimage docker-build-prod-baseimage
	cd python && docker buildx build -t ${KO_DOCKER_REPO}/${STORAGE_INIT_IMG} -f storage-initializer.Dockerfile .

docker-push-storageInitializer: docker-build-storageInitializer
	docker push ${KO_DOCKER_REPO}/${STORAGE_INIT_IMG}

docker-build-qpext:
	cd qpext && docker buildx build -t ${KO_DOCKER_REPO}/${QPEXT_IMG} -f qpext.Dockerfile .

docker-build-push-qpext: docker-build-qpext
	docker push ${KO_DOCKER_REPO}/${QPEXT_IMG}

test-qpext:
	cd qpext && go test -v ./... -cover

controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	test -s $(LOCALBIN)/controller-gen || GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)

KUSTOMIZE_INSTALL_SCRIPT ?= "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh"
.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	test -s $(LOCALBIN)/kustomize || { curl -Ss $(KUSTOMIZE_INSTALL_SCRIPT) | bash -s -- $(subst v,,$(KUSTOMIZE_VERSION)) $(LOCALBIN); }

envtest: $(ENVTEST) ## Download envtest-setup locally if necessary.
$(ENVTEST): $(LOCALBIN)
	test -s $(LOCALBIN)/setup-envtest || GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest

apidocs:
	docker buildx build -f docs/apis/Dockerfile --rm -t apidocs-gen . && \
	docker run -it --rm -v $(CURDIR)/pkg/apis:/go/src/github.com/kserve/kserve/pkg/apis -v ${PWD}/docs/apis:/go/gen-crd-api-reference-docs/apidocs apidocs-gen

.PHONY: check-doc-links
check-doc-links:
	@python3 hack/verify-doc-links.py && echo "$@: OK"
