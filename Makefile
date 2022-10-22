HAS_LINT := $(shell command -v golint;)

# Image URL to use all building/pushing image targets
IMG ?= kserve-controller:latest
AGENT_IMG ?= agent:latest
ROUTER_IMG ?= router:latest
SKLEARN_IMG ?= sklearnserver
XGB_IMG ?= xgbserver
LGB_IMG ?= lgbserver
PMML_IMG ?= pmmlserver
PADDLE_IMG ?= paddleserver
ALIBI_IMG ?= alibi-explainer
AIX_IMG ?= aix-explainer
STORAGE_INIT_IMG ?= storage-initializer
CRD_OPTIONS ?= "crd:maxDescLen=0"
KSERVE_ENABLE_SELF_SIGNED_CA ?= false
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.22

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KUSTOMIZE ?= $(LOCALBIN)/kustomize
ENVTEST ?= $(LOCALBIN)/setup-envtest
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen

## Tool Versions
KUSTOMIZE_VERSION ?= v3.8.7
CONTROLLER_TOOLS_VERSION ?= v0.4.0

# CPU/Memory limits for controller-manager
KSERVE_CONTROLLER_CPU_LIMIT ?= 100m
KSERVE_CONTROLLER_MEMORY_LIMIT ?= 300Mi
$(shell perl -pi -e 's/cpu:.*/cpu: $(KSERVE_CONTROLLER_CPU_LIMIT)/' config/default/manager_resources_patch.yaml)
$(shell perl -pi -e 's/memory:.*/memory: $(KSERVE_CONTROLLER_MEMORY_LIMIT)/' config/default/manager_resources_patch.yaml)

all: test manager agent router

# Run tests
test: fmt vet manifests envtest
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" go test -v $$(go list ./pkg/...) ./cmd/... -coverprofile coverage.out -coverpkg ./pkg/... ./cmd...

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
	kustomize edit remove resource certmanager/certificate.yaml; \
	else kustomize edit add resource certmanager/certificate.yaml; fi;
	kustomize build config/default | kubectl apply -f -
	if [ ${KSERVE_ENABLE_SELF_SIGNED_CA} != false ]; then ./hack/self-signed-ca.sh; fi;

deploy-dev: manifests
	./hack/image_patch_dev.sh development
	# Remove the certmanager certificate if KSERVE_ENABLE_SELF_SIGNED_CA is not false
	cd config/default && if [ ${KSERVE_ENABLE_SELF_SIGNED_CA} != false ]; then \
	kustomize edit remove resource certmanager/certificate.yaml; \
	else kustomize edit add resource certmanager/certificate.yaml; fi;
	kustomize build config/overlays/development | kubectl apply -f -
	# TODO: Add runtimes as part of default deployment
	kubectl wait --for=condition=ready pod -l control-plane=kserve-controller-manager -n kserve --timeout=300s
	kustomize build config/runtimes | kubectl apply -f -
	if [ ${KSERVE_ENABLE_SELF_SIGNED_CA} != false ]; then ./hack/self-signed-ca.sh; fi;

deploy-dev-sklearn: docker-push-sklearn
	./hack/model_server_patch_dev.sh sklearn ${KO_DOCKER_REPO}/${SKLEARN_IMG}
	kustomize build config/overlays/dev-image-config | kubectl apply -f -

deploy-dev-xgb: docker-push-xgb
	./hack/model_server_patch_dev.sh xgboost ${KO_DOCKER_REPO}/${XGB_IMG}
	kustomize build config/overlays/dev-image-config | kubectl apply -f -

deploy-dev-lgb: docker-push-lgb
	./hack/model_server_patch_dev.sh lightgbm ${KO_DOCKER_REPO}/${LGB_IMG}
	kustomize build config/overlays/dev-image-config | kubectl apply -f -

deploy-dev-pmml : docker-push-pmml
	./hack/model_server_patch_dev.sh sklearn ${KO_DOCKER_REPO}/${PMML_IMG}
	kustomize build config/overlays/dev-image-config | kubectl apply -f -

deploy-dev-paddle: docker-push-paddle
	./hack/model_server_patch_dev.sh paddle ${KO_DOCKER_REPO}/${PADDLE_IMG}
	kustomize build config/overlays/dev-image-config | kubectl apply -f -

deploy-dev-alibi: docker-push-alibi
	./hack/alibi_patch_dev.sh ${KO_DOCKER_REPO}/${ALIBI_IMG}
	kustomize build config/overlays/dev-image-config | kubectl apply -f -

deploy-dev-aix: docker-push-aix
	./hack/aix_patch_dev.sh ${KO_DOCKER_REPO}/${AIX_IMG}
	kustomize build config/overlays/dev-image-config | kubectl apply -f -

deploy-dev-storageInitializer: docker-push-storageInitializer
	./hack/storageInitializer_patch_dev.sh ${KO_DOCKER_REPO}/${STORAGE_INIT_IMG}
	kustomize build config/overlays/dev-image-config | kubectl apply -f -

deploy-ci: manifests
	kustomize build config/overlays/test | kubectl apply -f -
	# TODO: Add runtimes as part of default deployment
	kubectl wait --for=condition=ready pod -l control-plane=kserve-controller-manager -n kserve --timeout=300s
	kustomize build config/overlays/test/runtimes | kubectl apply -f -

undeploy:
	kustomize build config/default | kubectl delete -f -
	kubectl delete validatingwebhookconfigurations.admissionregistration.k8s.io inferenceservice.serving.kserve.io
	kubectl delete validatingwebhookconfigurations.admissionregistration.k8s.io trainedmodel.serving.kserve.io
	kubectl delete validatingwebhookconfigurations.admissionregistration.k8s.io inferencegraph.serving.kserve.io
	kubectl delete mutatingwebhookconfigurations.admissionregistration.k8s.io inferenceservice.serving.kserve.io

undeploy-dev:
	kustomize build config/overlays/development | kubectl delete -f -
	kubectl delete validatingwebhookconfigurations.admissionregistration.k8s.io inferenceservice.serving.kserve.io
	kubectl delete validatingwebhookconfigurations.admissionregistration.k8s.io trainedmodel.serving.kserve.io
	kubectl delete validatingwebhookconfigurations.admissionregistration.k8s.io inferencegraph.serving.kserve.io
	kubectl delete mutatingwebhookconfigurations.admissionregistration.k8s.io inferenceservice.serving.kserve.io

# Generate manifests e.g. CRD, RBAC etc.
manifests: controller-gen
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
	kustomize build config/crd > test/crds/serving.kserve.io_inferenceservices.yaml

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
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths=./pkg/apis/serving/v1alpha1
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths=./pkg/apis/serving/v1beta1
	hack/update-codegen.sh
	hack/update-openapigen.sh

# Build the docker image
docker-build: test
	docker build . -t ${IMG}
	@echo "updating kustomize image patch file for manager resource"

	# Use perl instead of sed to avoid OSX/Linux compatibility issue:
	# https://stackoverflow.com/questions/34533893/sed-command-creating-unwanted-duplicates-of-file-with-e-extension
	perl -pi -e 's@image: .*@image: '"${IMG}"'@' ./config/default/manager_image_patch.yaml

# Push the docker image
docker-push:
	docker push ${IMG}

docker-build-agent:
	docker build -f agent.Dockerfile . -t ${KO_DOCKER_REPO}/${AGENT_IMG}

docker-build-router:
	docker build -f router.Dockerfile . -t ${KO_DOCKER_REPO}/${ROUTER_IMG}

docker-push-agent:
	docker push ${KO_DOCKER_REPO}/${AGENT_IMG}

docker-push-router:
	docker push ${KO_DOCKER_REPO}/${ROUTER_IMG}

docker-build-sklearn:
	cd python && docker build -t ${KO_DOCKER_REPO}/${SKLEARN_IMG} -f sklearn.Dockerfile .

docker-push-sklearn: docker-build-sklearn
	docker push ${KO_DOCKER_REPO}/${SKLEARN_IMG}

docker-build-xgb:
	cd python && docker build -t ${KO_DOCKER_REPO}/${XGB_IMG} -f xgb.Dockerfile .

docker-push-xgb: docker-build-xgb
	docker push ${KO_DOCKER_REPO}/${XGB_IMG}

docker-build-lgb:
	cd python && docker build -t ${KO_DOCKER_REPO}/${LGB_IMG} -f lgb.Dockerfile .

docker-push-lgb: docker-build-lgb
	docker push ${KO_DOCKER_REPO}/${LGB_IMG}

docker-build-pmml:
	cd python && docker build -t ${KO_DOCKER_REPO}/${PMML_IMG} -f pmml.Dockerfile .

docker-push-pmml: docker-build-pmml
	docker push ${KO_DOCKER_REPO}/${PMML_IMG}

docker-build-paddle:
	cd python && docker build -t ${KO_DOCKER_REPO}/${PADDLE_IMG} -f paddle.Dockerfile .

docker-push-paddle: docker-build-paddle
	docker push ${KO_DOCKER_REPO}/${PADDLE_IMG}

docker-build-alibi:
	cd python && docker build -t ${KO_DOCKER_REPO}/${ALIBI_IMG} -f alibiexplainer.Dockerfile .

docker-push-alibi: docker-build-alibi
	docker push ${KO_DOCKER_REPO}/${ALIBI_IMG}

docker-build-aix:
	cd python && docker build -t ${KO_DOCKER_REPO}/${AIX_IMG} -f aixexplainer.Dockerfile .

docker-push-aix: docker-build-aix
	docker push ${KO_DOCKER_REPO}/${AIX_IMG}

docker-build-storageInitializer:
	cd python && docker build -t ${KO_DOCKER_REPO}/${STORAGE_INIT_IMG} -f storage-initializer.Dockerfile .

docker-push-storageInitializer: docker-build-storageInitializer
	docker push ${KO_DOCKER_REPO}/${STORAGE_INIT_IMG}

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
	docker build -f docs/apis/Dockerfile --rm -t apidocs-gen . && \
	docker run -it --rm -v $(CURDIR)/pkg/apis:/go/src/github.com/kserve/kserve/pkg/apis -v ${PWD}/docs/apis:/go/gen-crd-api-reference-docs/apidocs apidocs-gen

.PHONY: check-doc-links
check-doc-links:
	@python3 hack/verify-doc-links.py && echo "$@: OK"
