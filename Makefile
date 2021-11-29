HAS_LINT := $(shell command -v golint;)

# Image URL to use all building/pushing image targets
IMG ?= kserve-controller:latest
AGENT_IMG ?= agent:latest
SKLEARN_IMG ?= sklearnserver
XGB_IMG ?= xgbserver
LGB_IMG ?= lgbserver
PYTORCH_IMG ?= pytorchserver
PMML_IMG ?= pmmlserver
PADDLE_IMG ?= paddleserver
ALIBI_IMG ?= alibi-explainer
STORAGE_INIT_IMG ?= storage-initializer
CRD_OPTIONS ?= "crd:maxDescLen=0"
KSERVE_ENABLE_SELF_SIGNED_CA ?= false

# CPU/Memory limits for controller-manager
KSERVE_CONTROLLER_CPU_LIMIT ?= 100m
KSERVE_CONTROLLER_MEMORY_LIMIT ?= 300Mi
$(shell perl -pi -e 's/cpu:.*/cpu: $(KSERVE_CONTROLLER_CPU_LIMIT)/' config/default/manager_resources_patch.yaml)
$(shell perl -pi -e 's/memory:.*/memory: $(KSERVE_CONTROLLER_MEMORY_LIMIT)/' config/default/manager_resources_patch.yaml)

all: test manager agent

# Run tests
test: fmt vet manifests kubebuilder
	go test $$(go list ./pkg/... | grep -v /v1alpha2) ./cmd/... -coverprofile coverage.out

# Build manager binary
manager: generate fmt vet lint
	go build -o bin/manager ./cmd/manager

# Build agent binary
agent: fmt vet
	go build -o bin/agent ./cmd/agent

# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate fmt vet lint
	go run ./cmd/manager/main.go

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: manifests
	# Remove the certmanager certificate if KSERVE_ENABLE_SELF_SIGNED_CA is not false
	cd config/default && if [ ${KSERVE_ENABLE_SELF_SIGNED_CA} != false ]; then \
	kustomize edit remove resource certmanager/certificate.yaml; \
	else kustomize edit add resource certmanager/certificate.yaml; fi;
	kustomize build config/default | kubectl apply --validate=false -f -
	if [ ${KSERVE_ENABLE_SELF_SIGNED_CA} != false ]; then ./hack/self-signed-ca.sh; fi;

deploy-dev: manifests
	./hack/image_patch_dev.sh development
	# Remove the certmanager certificate if KSERVE_ENABLE_SELF_SIGNED_CA is not false
	cd config/default && if [ ${KSERVE_ENABLE_SELF_SIGNED_CA} != false ]; then \
	kustomize edit remove resource certmanager/certificate.yaml; \
	else kustomize edit add resource certmanager/certificate.yaml; fi;
	kustomize build config/overlays/development | kubectl apply --validate=false -f -
	# TODO: Add runtimes as part of default deployment
	kubectl wait --for=condition=ready pod -l control-plane=kserve-controller-manager -n kserve --timeout=300s
	kustomize build config/runtimes | kubectl apply --validate=false -f -
	if [ ${KSERVE_ENABLE_SELF_SIGNED_CA} != false ]; then ./hack/self-signed-ca.sh; fi;

deploy-dev-sklearn: docker-push-sklearn
	./hack/model_server_patch_dev.sh sklearn ${KO_DOCKER_REPO}/${SKLEARN_IMG}
	kustomize build config/overlays/dev-image-config | kubectl apply --validate=false -f -

deploy-dev-xgb: docker-push-xgb
	./hack/model_server_patch_dev.sh xgboost ${KO_DOCKER_REPO}/${XGB_IMG}
	kustomize build config/overlays/dev-image-config | kubectl apply --validate=false -f -

deploy-dev-lgb: docker-push-lgb
	./hack/model_server_patch_dev.sh lightgbm ${KO_DOCKER_REPO}/${LGB_IMG}
	kustomize build config/overlays/dev-image-config | kubectl apply --validate=false -f -

deploy-dev-pytorch: docker-push-pytorch
	./hack/model_server_patch_dev.sh pytorch ${KO_DOCKER_REPO}/${PYTORCH_IMG}
	kustomize build config/overlays/dev-image-config | kubectl apply --validate=false -f -

deploy-dev-pmml : docker-push-pmml
	./hack/model_server_patch_dev.sh sklearn ${KO_DOCKER_REPO}/${PMML_IMG}
	kustomize build config/overlays/dev-image-config | kubectl apply --validate=false -f -

deploy-dev-paddle: docker-push-paddle
	./hack/model_server_patch_dev.sh paddle ${KO_DOCKER_REPO}/${PADDLE_IMG}
	kustomize build config/overlays/dev-image-config | kubectl apply --validate=false -f -

deploy-dev-alibi: docker-push-alibi
	./hack/alibi_patch_dev.sh ${KO_DOCKER_REPO}/${ALIBI_IMG}
	kustomize build config/overlays/dev-image-config | kubectl apply --validate=false -f -

deploy-dev-storageInitializer: docker-push-storageInitializer
	./hack/storageInitializer_patch_dev.sh ${KO_DOCKER_REPO}/${STORAGE_INIT_IMG}
	kustomize build config/overlays/dev-image-config | kubectl apply --validate=false -f -

deploy-ci: manifests
	kustomize build config/overlays/test | kubectl apply -f -
	# TODO: Add runtimes as part of default deployment
	kubectl wait --for=condition=ready pod -l control-plane=kserve-controller-manager -n kserve --timeout=300s
	kustomize build config/runtimes | kubectl apply --validate=false -f -

undeploy:
	kustomize build config/default | kubectl delete -f -
	kubectl delete validatingwebhookconfigurations.admissionregistration.k8s.io inferenceservice.serving.kserve.io
	kubectl delete validatingwebhookconfigurations.admissionregistration.k8s.io trainedmodel.serving.kserve.io
	kubectl delete mutatingwebhookconfigurations.admissionregistration.k8s.io inferenceservice.serving.kserve.io

undeploy-dev:
	kustomize build config/overlays/development | kubectl delete -f -
	kubectl delete validatingwebhookconfigurations.admissionregistration.k8s.io inferenceservice.serving.kserve.io
	kubectl delete validatingwebhookconfigurations.admissionregistration.k8s.io trainedmodel.serving.kserve.io
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
	#remove the required property on framework as name field needs to be optional
	yq d -i config/crd/serving.kserve.io_inferenceservices.yaml 'spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.*.properties.*.required'
	#remove ephemeralContainers properties for compress crd size https://github.com/kubeflow/kfserving/pull/1141#issuecomment-714170602
	yq d -i config/crd/serving.kserve.io_inferenceservices.yaml 'spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.*.properties.ephemeralContainers'
	yq d -i config/crd/serving.kserve.io_inferenceservices.yaml 'spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.*.properties.initContainers'
	#knative does not allow setting port on liveness or readiness probe
	yq d -i config/crd/serving.kserve.io_inferenceservices.yaml 'spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.*.properties.*.properties.readinessProbe.properties.httpGet.required'
	yq d -i config/crd/serving.kserve.io_inferenceservices.yaml 'spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.*.properties.*.properties.livenessProbe.properties.httpGet.required'
	yq d -i config/crd/serving.kserve.io_inferenceservices.yaml 'spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.*.properties.*.properties.readinessProbe.properties.tcpSocket.required'
	yq d -i config/crd/serving.kserve.io_inferenceservices.yaml 'spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.*.properties.*.properties.livenessProbe.properties.tcpSocket.required'
	yq d -i config/crd/serving.kserve.io_inferenceservices.yaml 'spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.*.properties.containers.items.properties.livenessProbe.properties.httpGet.required'
	yq d -i config/crd/serving.kserve.io_inferenceservices.yaml 'spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.*.properties.containers.items.properties.readinessProbe.properties.httpGet.required'
	#With v1 and newer kubernetes protocol requires default
	yq read config/crd/serving.kserve.io_inferenceservices.yaml spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.**.protocol -p p | awk '{print $$0".default"}' | xargs -n1 -I{} yq w -i config/crd/serving.kserve.io_inferenceservices.yaml {} TCP
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
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths=./pkg/apis/serving/v1alpha1
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths=./pkg/apis/serving/v1beta1
	#TODO update-codegen.sh is not used and requires vendor
	#hack/update-codegen.sh
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

docker-push-agent:
	docker push ${KO_DOCKER_REPO}/${AGENT_IMG}

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

docker-build-pytorch:
	cd python && docker build -t ${KO_DOCKER_REPO}/${PYTORCH_IMG} -f pytorch.Dockerfile .

docker-push-pytorch: docker-build-pytorch
	docker push ${KO_DOCKER_REPO}/${PYTORCH_IMG}

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

docker-build-storageInitializer:
	cd python && docker build -t ${KO_DOCKER_REPO}/${STORAGE_INIT_IMG} -f storage-initializer.Dockerfile .

docker-push-storageInitializer: docker-build-storageInitializer
	docker push ${KO_DOCKER_REPO}/${STORAGE_INIT_IMG}

kubebuilder:
ifeq (, $(shell which kubebuilder))
	@{ \
	set -e ;\
        os=$$(go env GOOS) ; \
        arch=$$(go env GOARCH) ;\
	curl -L -o kubebuilder https://go.kubebuilder.io/dl/latest/$${os}/$${arch} ;\
	chmod +x kubebuilder && sudo mv kubebuilder /usr/local/bin ;\
	sudo mkdir -p /usr/local/kubebuilder ;\
	curl -sSLo envtest-bins.tar.gz "https://storage.googleapis.com/kubebuilder-tools/kubebuilder-tools-1.19.2-$${os}-$${arch}.tar.gz" ;\
	sudo tar -C /usr/local/kubebuilder --strip-components=1 -zvxf envtest-bins.tar.gz ;\
	}
endif

controller-gen:
ifeq (, $(shell which controller-gen))
	@{ \
	set -e ;\
	CONTROLLER_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$CONTROLLER_GEN_TMP_DIR ;\
	go mod init tmp ;\
	go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.4.0 ;\
	rm -rf $$CONTROLLER_GEN_TMP_DIR ;\
	}
CONTROLLER_GEN=$(GOPATH)/bin/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif

apidocs:
	docker build -f docs/apis/Dockerfile --rm -t apidocs-gen . && \
	docker run -it --rm -v $(CURDIR)/pkg/apis:/go/src/github.com/kserve/kserve/pkg/apis -v ${PWD}/docs/apis:/go/gen-crd-api-reference-docs/apidocs apidocs-gen
