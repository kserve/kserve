HAS_LINT := $(shell command -v golint;)

# Image URL to use all building/pushing image targets
IMG ?= kfserving-controller:latest
LOGGER_IMG ?= logger:latest
BATCHER_IMG ?= batcher:latest
SKLEARN_IMG ?= sklearnserver:latest
XGB_IMG ?= xgbserver:latest
PYTORCH_IMG ?= pytorchserver:latest
ALIBI_IMG ?= alibi-explainer:latest
STORAGE_INIT_IMG ?= storage-initializer:latest
CRD_OPTIONS ?= "crd:maxDescLen=0"
KFSERVING_ENABLE_SELF_SIGNED_CA ?= false

# CPU/Memory limits for controller-manager
KFSERVING_CONTROLLER_CPU_LIMIT ?= 100m
KFSERVING_CONTROLLER_MEMORY_LIMIT ?= 300Mi
$(shell perl -pi -e 's/cpu:.*/cpu: $(KFSERVING_CONTROLLER_CPU_LIMIT)/' config/default/manager_resources_patch.yaml)
$(shell perl -pi -e 's/memory:.*/memory: $(KFSERVING_CONTROLLER_MEMORY_LIMIT)/' config/default/manager_resources_patch.yaml)

all: test manager logger batcher

# Run tests
test: fmt vet manifests kubebuilder
	go test ./pkg/... ./cmd/... -coverprofile coverage.out

# Build manager binary
manager: generate fmt vet lint
	go build -o bin/manager ./cmd/manager

# Build logger binary
logger: fmt vet
	go build -o bin/logger ./cmd/logger

# Build batcher binary
batcher: fmt vet
	go build -o bin/batcher ./cmd/batcher

# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate fmt vet lint
	go run ./cmd/manager/main.go

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: manifests
	# Remove the certmanager certificate if KFSERVING_ENABLE_SELF_SIGNED_CA is not false
	cd config/default && if [ ${KFSERVING_ENABLE_SELF_SIGNED_CA} != false ]; then \
	kustomize edit remove resource certmanager/certificate.yaml; \
	else kustomize edit add resource certmanager/certificate.yaml; fi;

	kustomize build config/default | kubectl apply --validate=false -f -
	if [ ${KFSERVING_ENABLE_SELF_SIGNED_CA} != false ]; then ./hack/self-signed-ca.sh; fi;

deploy-dev: manifests
	./hack/image_patch_dev.sh development
	# Remove the certmanager certificate if KFSERVING_ENABLE_SELF_SIGNED_CA is not false
	cd config/default && if [ ${KFSERVING_ENABLE_SELF_SIGNED_CA} != false ]; then \
	kustomize edit remove resource certmanager/certificate.yaml; \
	else kustomize edit add resource certmanager/certificate.yaml; fi;

	kustomize build config/overlays/development | kubectl apply --validate=false -f -
	if [ ${KFSERVING_ENABLE_SELF_SIGNED_CA} != false ]; then ./hack/self-signed-ca.sh; fi;

deploy-dev-sklearn: docker-push-sklearn
	./hack/model_server_patch_dev.sh sklearn ${KO_DOCKER_REPO}/${SKLEARN_IMG}
	kustomize build config/overlays/dev-image-config | kubectl apply --validate=false -f -

deploy-dev-xgb: docker-push-xgb
	./hack/model_server_patch_dev.sh xgboost ${KO_DOCKER_REPO}/${XGB_IMG}
	kustomize build config/overlays/dev-image-config | kubectl apply --validate=false -f -

deploy-dev-pytorch: docker-push-pytorch
	./hack/model_server_patch_dev.sh pytorch ${KO_DOCKER_REPO}/${PYTORCH_IMG}
	kustomize build config/overlays/dev-image-config | kubectl apply --validate=false -f -

deploy-dev-alibi: docker-push-alibi
	./hack/alibi_patch_dev.sh ${KO_DOCKER_REPO}/${ALIBI_IMG}
	kustomize build config/overlays/dev-image-config | kubectl apply --validate=false -f -

deploy-dev-storageInitializer: docker-push-storageInitializer
	./hack/misc_patch_dev.sh storageInitializer ${KO_DOCKER_REPO}/${STORAGE_INIT_IMG}
	kustomize build config/overlays/dev-image-config | kubectl apply --validate=false -f -

deploy-ci: manifests
	kustomize build config/overlays/test | kubectl apply -f -

undeploy:
	kustomize build config/default | kubectl delete -f -
	kubectl delete validatingwebhookconfigurations.admissionregistration.k8s.io inferenceservice.serving.kubeflow.org
	kubectl delete mutatingwebhookconfigurations.admissionregistration.k8s.io inferenceservice.serving.kubeflow.org

undeploy-dev:
	kustomize build config/overlays/development | kubectl delete -f -
	kubectl delete validatingwebhookconfigurations.admissionregistration.k8s.io inferenceservice.serving.kubeflow.org
	kubectl delete mutatingwebhookconfigurations.admissionregistration.k8s.io inferenceservice.serving.kubeflow.org

# Generate manifests e.g. CRD, RBAC etc.
manifests: controller-gen
	$(CONTROLLER_GEN) $(CRD_OPTIONS) paths=./pkg/apis/serving/... output:crd:dir=config/crd
	$(CONTROLLER_GEN) rbac:roleName=kfserving-manager-role paths=./pkg/controller/... output:rbac:artifacts:config=config/rbac
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths=./pkg/apis/serving/v1alpha2
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths=./pkg/apis/serving/v1beta1
	#TODO Remove this until new controller-tools is released
	perl -pi -e 's/storedVersions: null/storedVersions: []/g' config/crd/serving.kubeflow.org_inferenceservices.yaml
	perl -pi -e 's/conditions: null/conditions: []/g' config/crd/serving.kubeflow.org_inferenceservices.yaml
	perl -pi -e 's/Any/string/g' config/crd/serving.kubeflow.org_inferenceservices.yaml
	perl -pi -e 's/storedVersions: null/storedVersions: []/g' config/crd/serving.kubeflow.org_trainedmodels.yaml
	perl -pi -e 's/conditions: null/conditions: []/g' config/crd/serving.kubeflow.org_trainedmodels.yaml
	perl -pi -e 's/Any/string/g' config/crd/serving.kubeflow.org_trainedmodels.yaml
	#TODO v1beta1 crd openAPIV3Schema is too big and kubectl client side apply takes long time to do diffs, need to use k8s 1.18's server side apply
	#https://kubernetes.io/blog/2020/04/01/kubernetes-1.18-feature-server-side-apply-beta-2/#what-is-server-side-apply
	#remove the required property on framework as name field needs to be optional
	yq d -i config/crd/serving.kubeflow.org_inferenceservices.yaml 'spec.versions[1].schema.openAPIV3Schema.properties.spec.properties.*.properties.*.required'
	#knative does not allow setting port on liveness or readiness probe
	yq d -i config/crd/serving.kubeflow.org_inferenceservices.yaml 'spec.versions[1].schema.openAPIV3Schema.properties.spec.properties.*.properties.*.properties.readinessProbe.properties.httpGet.required'
	yq d -i config/crd/serving.kubeflow.org_inferenceservices.yaml 'spec.versions[1].schema.openAPIV3Schema.properties.spec.properties.*.properties.*.properties.livenessProbe.properties.httpGet.required'
	yq d -i config/crd/serving.kubeflow.org_inferenceservices.yaml 'spec.versions[1].schema.openAPIV3Schema.properties.spec.properties.*.properties.*.properties.readinessProbe.properties.tcpSocket.required'
	yq d -i config/crd/serving.kubeflow.org_inferenceservices.yaml 'spec.versions[1].schema.openAPIV3Schema.properties.spec.properties.*.properties.*.properties.livenessProbe.properties.tcpSocket.required'


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
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths=./pkg/apis/serving/v1alpha2
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

docker-build-logger: test
	docker build -f logger.Dockerfile . -t ${LOGGER_IMG}

docker-push-logger:
	docker push ${LOGGER_IMG}

docker-build-batcher:
	docker build -f batcher.Dockerfile . -t ${BATCHER_IMG}

docker-push-batcher:
	docker push ${BATCHER_IMG}

docker-build-sklearn: 
	cd python && docker build -t ${KO_DOCKER_REPO}/${SKLEARN_IMG} -f sklearn.Dockerfile .

docker-push-sklearn: docker-build-sklearn
	docker push ${KO_DOCKER_REPO}/${SKLEARN_IMG}

docker-build-xgb: 
	cd python && docker build -t ${KO_DOCKER_REPO}/${XGB_IMG} -f xgb.Dockerfile .

docker-push-xgb: docker-build-xgb
	docker push ${KO_DOCKER_REPO}/${XGB_IMG}

docker-build-pytorch: 
	cd python && docker build -t ${KO_DOCKER_REPO}/${PYTORCH_IMG} -f pytorch.Dockerfile .

docker-push-pytorch: docker-build-pytorch
	docker push ${KO_DOCKER_REPO}/${PYTORCH_IMG}

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
        curl -L "https://go.kubebuilder.io/dl/2.3.1/$${os}/$${arch}" | tar -xz -C /tmp/ ;\
        sudo mkdir -p /usr/local/kubebuilder/bin/ ;\
	sudo mv /tmp/kubebuilder_2.3.1_$${os}_$${arch}/bin/* /usr/local/kubebuilder/bin/ ;\
	echo "Add /usr/local/kubebuilder/bin/ to your PATH!!" ;\
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
	docker run -it --rm -v ${GOPATH}/src/github.com/kubeflow/kfserving/pkg/apis:/go/src/github.com/kubeflow/kfserving/pkg/apis -v ${PWD}/docs/apis:/go/gen-crd-api-reference-docs/apidocs apidocs-gen
