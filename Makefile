HAS_LINT := $(shell command -v golint;)


# Image URL to use all building/pushing image targets
IMG ?= kfserving-controller:latest
EXECUTOR_IMG ?= kfserving-executor:latest

all: test manager executor

# Run tests
test: generate fmt vet lint manifests
	go test ./pkg/... ./cmd/... -coverprofile cover.out

# Build manager binary
manager: generate fmt vet lint
	go build -o bin/manager ./cmd/manager

# Build manager binary
logger: fmt vet
	go build -o bin/logger ./cmd/logger

# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate fmt vet lint
	go run ./cmd/manager/main.go

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: manifests
	kustomize build config/default | kubectl apply -f -

deploy-dev: manifests
	./hack/image_patch_dev.sh
	kustomize build config/overlays/development | kubectl apply -f -

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
manifests:
	go run vendor/sigs.k8s.io/controller-tools/cmd/controller-gen/main.go crd --output-dir=config/default/crds
	kustomize build config/default/crds -o config/default/crds/serving_v1alpha2_inferenceservice.yaml
	go run vendor/sigs.k8s.io/controller-tools/cmd/controller-gen/main.go rbac --output-dir=config/default/rbac

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
generate:
ifndef GOPATH
	$(error GOPATH not defined, please define GOPATH. Run "go help gopath" to learn more about GOPATH)
endif
	go generate ./pkg/... ./cmd/...
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

docker-build-executor: test
	docker build -f executor.Dockerfile . -t ${EXECUTOR_IMG}

docker-push-executor:
	docker push ${EXECUTOR_IMG}

apidocs:
	docker build -f docs/apis/Dockerfile --rm -t apidocs-gen . && \
	docker run -it --rm -v ${GOPATH}/src/github.com/kubeflow/kfserving/pkg/apis:/go/src/github.com/kubeflow/kfserving/pkg/apis -v ${PWD}/docs/apis:/go/gen-crd-api-reference-docs/apidocs apidocs-gen
