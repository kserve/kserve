# The Go and Python based tools are defined in Makefile.tools.mk.
include Makefile.tools.mk

# Load dependency versions
include kserve-deps.env

# Load image configurations
include kserve-images.env

# Base Image URL
BASE_IMG ?= python:3.11-slim-bookworm
PMML_BASE_IMG ?= eclipse-temurin:21-jdk-noble

CRD_OPTIONS ?= "crd:maxDescLen=0"
KSERVE_ENABLE_SELF_SIGNED_CA ?= false

ENVTEST ?= $(LOCALBIN)/setup-envtest
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_VERSION ?= $(shell go list -m -f "{{ .Version }}" sigs.k8s.io/controller-runtime | awk -F'[v.]' '{printf "release-%d.%d", $$2, $$3}')
ENVTEST_K8S_VERSION ?= $(shell go list -m -f "{{ .Version }}" k8s.io/api | awk -F'[v.]' '{printf "1.%d", $$3}')

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

.PHONY: setup-envtest
setup-envtest: envtest
	@echo "Setting up envtest binaries for Kubernetes version $(ENVTEST_K8S_VERSION)..."
	@$(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path || { \
		echo "Error: Failed to set up envtest binaries for version $(ENVTEST_K8S_VERSION)."; \
		exit 1; \
		}

# Run go fmt against code
fmt:
	go fmt ./pkg/... ./cmd/... && cd qpext && go fmt ./...

py-fmt: $(BLACK_FMT)
	$(BLACK_FMT) --config python/pyproject.toml ./python ./docs

# Run go vet against code
vet:
	go vet ./pkg/... ./cmd/... && cd qpext && go vet ./...

tidy:
	go mod tidy
	cd qpext && go mod tidy

.PHONY: sync-deps
sync-deps:
	@@python3 hack/setup/scripts/generate-versions-from-gomod.py

.PHONY: sync-img-env
sync-img-env:
	@python3 hack/setup/scripts/generate-images-sh.py

go-lint: golangci-lint
	@$(GOLANGCI_LINT) run --fix

py-lint: $(RUFF)
	$(RUFF) check --config ruff.toml 

validate-infra-scripts:
	@python3 hack/setup/scripts/validate-install-scripts.py

generate-quick-install-scripts: validate-infra-scripts $(PYTHON_VENV)
	@$(PYTHON_BIN)/pip install -q -r hack/setup/scripts/install-script-generator/requirements.txt
	@$(PYTHON_BIN)/python hack/setup/scripts/install-script-generator/generator.py

# Generate manifests e.g. CRD, RBAC etc.
manifests: controller-gen yq
	@$(CONTROLLER_GEN) $(CRD_OPTIONS) paths=./pkg/apis/serving/... output:crd:dir=config/crd/full	
	@$(CONTROLLER_GEN) rbac:roleName=kserve-manager-role paths={./pkg/controller/v1alpha1/inferencegraph,./pkg/controller/v1alpha1/trainedmodel,./pkg/controller/v1beta1/...} output:rbac:artifacts:config=config/rbac
	@$(CONTROLLER_GEN) rbac:roleName=kserve-llmisvc-manager-role paths=./pkg/controller/v1alpha2/llmisvc output:rbac:artifacts:config=config/rbac/llmisvc
	@$(CONTROLLER_GEN) rbac:roleName=kserve-localmodel-manager-role paths=./pkg/controller/v1alpha1/localmodel output:rbac:artifacts:config=config/rbac/localmodel
	@$(CONTROLLER_GEN) rbac:roleName=kserve-localmodelnode-agent-role paths=./pkg/controller/v1alpha1/localmodelnode output:rbac:artifacts:config=config/rbac/localmodelnode
	
	# Move LLMISVC CRD to llmisvc folder	                   
	mv config/crd/full/serving.kserve.io_llminferenceservices.yaml config/crd/full/llmisvc/serving.kserve.io_llminferenceservices.yaml
	mv config/crd/full/serving.kserve.io_llminferenceserviceconfigs.yaml config/crd/full/llmisvc/serving.kserve.io_llminferenceserviceconfigs.yaml
	
	# Move LocalModel CRD to localmodel folder
	mv config/crd/full/serving.kserve.io_localmodelcaches.yaml config/crd/full/localmodel/serving.kserve.io_localmodelcaches.yaml
	mv config/crd/full/serving.kserve.io_localmodelnodegroups.yaml config/crd/full/localmodel/serving.kserve.io_localmodelnodegroups.yaml
	mv config/crd/full/serving.kserve.io_localmodelnodes.yaml config/crd/full/localmodel/serving.kserve.io_localmodelnodes.yaml
	
	# Copy the cluster role to the helm chart
	cp config/rbac/auth_proxy_role.yaml charts/kserve-resources/templates/clusterrole.yaml
	cat config/rbac/role.yaml >> charts/kserve-resources/templates/clusterrole.yaml
	# Copy the llmisvc cluster role to the helm chart
	cat config/rbac/llmisvc/role.yaml > charts/kserve-llmisvc-resources/templates/clusterrole.yaml
	cat config/rbac/llmisvc/leader_election_role.yaml > charts/kserve-llmisvc-resources/templates/leader_election_role.yaml	
	# Copy the local model role with Helm chart while keeping the Helm template condition
	echo '{{- if .Values.kserve.localmodel.enabled }}' > charts/kserve-resources/templates/localmodel/role.yaml
	cat config/rbac/localmodel/role.yaml >> charts/kserve-resources/templates/localmodel/role.yaml
	echo '{{- end }}' >> charts/kserve-resources/templates/localmodel/role.yaml
	# Copy the local model node role with Helm chart while keeping the Helm template condition
	echo '{{- if .Values.kserve.localmodel.enabled }}'> charts/kserve-resources/templates/localmodelnode/role.yaml
	cat config/rbac/localmodelnode/role.yaml >> charts/kserve-resources/templates/localmodelnode/role.yaml
	echo '{{- end }}' >> charts/kserve-resources/templates/localmodelnode/role.yaml

	@$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths=./pkg/apis/serving/v1alpha1
	@$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths=./pkg/apis/serving/v1alpha2
	@$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths=./pkg/apis/serving/v1beta1

	# Remove validation for the LLMInferenceServiceConfig API so that we can use Go templates to inject values at runtime.
	# Note: v1alpha1 is at index 0, v1alpha2 is at index 1. These rules target v1alpha2 which has the full InferencePoolSpec.
	@$(YQ) 'del(.spec.versions[1].schema.openAPIV3Schema.properties.spec.properties.router.properties.route.properties.http.properties.spec.properties.rules.items.properties.matches.items.properties.path.x-kubernetes-validations)' -i config/crd/full/llmisvc/serving.kserve.io_llminferenceserviceconfigs.yaml
	@$(YQ) 'del(.spec.versions[1].schema.openAPIV3Schema.properties.spec.properties.router.properties.route.properties.http.properties.spec.properties.rules.items.properties.filters.items.properties.urlRewrite.properties.path.x-kubernetes-validations)' -i config/crd/full/llmisvc/serving.kserve.io_llminferenceserviceconfigs.yaml
	@$(YQ) 'del(.spec.versions[1].schema.openAPIV3Schema.properties.spec.properties.router.properties.route.properties.http.properties.spec.properties.parentRefs.items.properties.namespace.pattern)' -i config/crd/full/llmisvc/serving.kserve.io_llminferenceserviceconfigs.yaml
	# Remove pattern validation from InferencePool selector matchLabels to allow Go templates
	@$(YQ) 'del(.spec.versions[1].schema.openAPIV3Schema.properties.spec.properties.router.properties.scheduler.properties.pool.properties.spec.properties.selector.properties.matchLabels.additionalProperties.pattern)' -i config/crd/full/llmisvc/serving.kserve.io_llminferenceserviceconfigs.yaml
	@$(YQ) 'del(.spec.versions[1].schema.openAPIV3Schema.properties.spec.properties.router.properties.scheduler.properties.pool.properties.spec.properties.selector.properties.matchLabels.additionalProperties.maxLength)' -i config/crd/full/llmisvc/serving.kserve.io_llminferenceserviceconfigs.yaml
	@$(YQ) 'del(.spec.versions[1].schema.openAPIV3Schema.properties.spec.properties.router.properties.scheduler.properties.pool.properties.spec.properties.selector.properties.matchLabels.additionalProperties.minLength)' -i config/crd/full/llmisvc/serving.kserve.io_llminferenceserviceconfigs.yaml
	@$(YQ) 'del(.spec.versions[1].schema.openAPIV3Schema.properties.spec.properties.router.properties.scheduler.properties.pool.properties.spec.properties.selector.properties.matchLabels.maxProperties)' -i config/crd/full/llmisvc/serving.kserve.io_llminferenceserviceconfigs.yaml
	@$(YQ) 'del(.spec.versions[1].schema.openAPIV3Schema.properties.spec.properties.router.properties.scheduler.properties.pool.properties.spec.properties.selector.properties.matchLabels.minProperties)' -i config/crd/full/llmisvc/serving.kserve.io_llminferenceserviceconfigs.yaml
	# Remove validation for the LLMInferenceServiceConfig API so that we can override only specific values (both versions).
	@$(YQ) 'del(.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.worker.required)' -i config/crd/full/llmisvc/serving.kserve.io_llminferenceserviceconfigs.yaml
	@$(YQ) 'del(.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.template.required)' -i config/crd/full/llmisvc/serving.kserve.io_llminferenceserviceconfigs.yaml
	@$(YQ) 'del(.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.prefill.properties.template.required)' -i config/crd/full/llmisvc/serving.kserve.io_llminferenceserviceconfigs.yaml
	@$(YQ) 'del(.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.prefill.properties.worker.required)' -i config/crd/full/llmisvc/serving.kserve.io_llminferenceserviceconfigs.yaml
	@$(YQ) 'del(.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.router.properties.scheduler.properties.template.required)' -i config/crd/full/llmisvc/serving.kserve.io_llminferenceserviceconfigs.yaml
	@$(YQ) 'del(.spec.versions[1].schema.openAPIV3Schema.properties.spec.properties.worker.required)' -i config/crd/full/llmisvc/serving.kserve.io_llminferenceserviceconfigs.yaml
	@$(YQ) 'del(.spec.versions[1].schema.openAPIV3Schema.properties.spec.properties.template.required)' -i config/crd/full/llmisvc/serving.kserve.io_llminferenceserviceconfigs.yaml
	@$(YQ) 'del(.spec.versions[1].schema.openAPIV3Schema.properties.spec.properties.prefill.properties.template.required)' -i config/crd/full/llmisvc/serving.kserve.io_llminferenceserviceconfigs.yaml
	@$(YQ) 'del(.spec.versions[1].schema.openAPIV3Schema.properties.spec.properties.prefill.properties.worker.required)' -i config/crd/full/llmisvc/serving.kserve.io_llminferenceserviceconfigs.yaml
	@$(YQ) 'del(.spec.versions[1].schema.openAPIV3Schema.properties.spec.properties.router.properties.scheduler.properties.template.required)' -i config/crd/full/llmisvc/serving.kserve.io_llminferenceserviceconfigs.yaml
	# Remove validation for the LLMInferenceService API so that we can override only specific values (both versions).
	@$(YQ) 'del(.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.worker.required)' -i config/crd/full/llmisvc/serving.kserve.io_llminferenceservices.yaml
	@$(YQ) 'del(.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.template.required)' -i config/crd/full/llmisvc/serving.kserve.io_llminferenceservices.yaml
	@$(YQ) 'del(.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.prefill.properties.template.required)' -i config/crd/full/llmisvc/serving.kserve.io_llminferenceservices.yaml
	@$(YQ) 'del(.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.prefill.properties.worker.required)' -i config/crd/full/llmisvc/serving.kserve.io_llminferenceservices.yaml
	@$(YQ) 'del(.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.router.properties.scheduler.properties.template.required)' -i config/crd/full/llmisvc/serving.kserve.io_llminferenceservices.yaml
	@$(YQ) 'del(.spec.versions[1].schema.openAPIV3Schema.properties.spec.properties.worker.required)' -i config/crd/full/llmisvc/serving.kserve.io_llminferenceservices.yaml
	@$(YQ) 'del(.spec.versions[1].schema.openAPIV3Schema.properties.spec.properties.template.required)' -i config/crd/full/llmisvc/serving.kserve.io_llminferenceservices.yaml
	@$(YQ) 'del(.spec.versions[1].schema.openAPIV3Schema.properties.spec.properties.prefill.properties.template.required)' -i config/crd/full/llmisvc/serving.kserve.io_llminferenceservices.yaml
	@$(YQ) 'del(.spec.versions[1].schema.openAPIV3Schema.properties.spec.properties.prefill.properties.worker.required)' -i config/crd/full/llmisvc/serving.kserve.io_llminferenceservices.yaml
	@$(YQ) 'del(.spec.versions[1].schema.openAPIV3Schema.properties.spec.properties.router.properties.scheduler.properties.template.required)' -i config/crd/full/llmisvc/serving.kserve.io_llminferenceservices.yaml

	# DO NOT COPY to helm chart. It needs to be created before the Envoy Gateway or you will need to restart the Envoy Gateway controller.
	# The llmisvc helm chart needs to be installed after the Envoy Gateway as well, so it needs to be created before the llmisvc helm chart.
	kubectl kustomize https://github.com/kubernetes-sigs/gateway-api-inference-extension.git/config/crd?ref=$(GIE_VERSION) > test/crds/gateway-inference-extension.yaml

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
	kubectl kustomize config/crd/full > test/crds/serving.kserve.io_all_crds.yaml
	echo "---" >> test/crds/serving.kserve.io_all_crds.yaml
	kubectl kustomize config/crd/full/llmisvc >> test/crds/serving.kserve.io_all_crds.yaml
	echo "---" >> test/crds/serving.kserve.io_all_crds.yaml
	kubectl kustomize config/crd/full/localmodel >> test/crds/serving.kserve.io_all_crds.yaml
	
	# Copy the minimal crd to the helm chart
	cp config/crd/minimal/*.yaml charts/kserve-crd-minimal/templates/
	cp config/crd/minimal/llmisvc/*.yaml charts/kserve-llmisvc-crd-minimal/templates/
	cp -f config/crd/minimal/localmodel/*.yaml charts/kserve-crd-minimal/templates/
	cp -f config/crd/minimal/localmodel/*.yaml charts/kserve-llmisvc-crd-minimal/templates/
	rm charts/kserve-crd-minimal/templates/kustomization.yaml
	rm charts/kserve-llmisvc-crd-minimal/templates/kustomization.yaml

	# Copy the full crd to the helm chart
	cp config/crd/full/*.yaml charts/kserve-crd/templates/
	# Copy llmisvc crd (with conversion webhook patches applied via kustomize)
	kubectl kustomize config/crd/full/llmisvc | $(YQ) 'select(.metadata.name == "llminferenceservices.serving.kserve.io")' > charts/kserve-llmisvc-crd/templates/serving.kserve.io_llminferenceservices.yaml
	kubectl kustomize config/crd/full/llmisvc | $(YQ) 'select(.metadata.name == "llminferenceserviceconfigs.serving.kserve.io")' > charts/kserve-llmisvc-crd/templates/serving.kserve.io_llminferenceserviceconfigs.yaml
	cp -f config/crd/full/localmodel/*.yaml charts/kserve-crd/templates/
	cp -f config/crd/full/localmodel/*.yaml charts/kserve-llmisvc-crd/templates/
	rm charts/kserve-crd/templates/kustomization.yaml
	rm charts/kserve-llmisvc-crd/templates/kustomization.yaml
    # Copy Test inferenceconfig configmap to test overlay
	cp config/configmap/inferenceservice.yaml config/overlays/test/configmap/inferenceservice.yaml

# Generate code
generate: controller-gen helm-docs
	hack/update-codegen.sh
	hack/update-openapigen.sh
	hack/python-sdk/client-gen.sh
	$(HELM_DOCS) --chart-search-root=charts --output-file=README.md

# Update uv.lock files
uv-lock: $(UV)
# Update the kserve package first as other packages depends on it.
	cd ./python && \
	cd kserve && $(UV) lock && cd .. && \
	for file in $$(find . -type f -name "pyproject.toml" -not -path "./pyproject.toml" -not -path "*.venv/*"); do \
		folder=$$(dirname "$$file"); \
		echo "moving into folder $$folder"; \
		case "$$folder" in \
			*plugin*|plugin|kserve) \
				echo -e "\033[33mSkipping folder $$folder\033[0m" ;; \
			*) \
				cd "$$folder" && $(UV) lock && cd - > /dev/null ;; \
		esac; \
	done

.PHONY: ensure-go-version-upgrade ensure-golangci-go-version
ensure-go-version-upgrade: ensure-golangci-go-version

ensure-golangci-go-version: yq	
	@GO_GOMOD_VERSION="$$(grep -m1 '^go ' go.mod | cut -d' ' -f2 | cut -d. -f1-2)"; \
	GO_GOLANGCI_VERSION="$$($(YQ) -r '.run.go // ""' .golangci.yml | cut -d. -f1-2)"; \
	if [ -z "$${GO_GOLANGCI_VERSION}" ]; then \
		echo "INFO: '.golangci.yml:run.go' is not set; defaulting to $$GO_GOMOD_VERSION."; \
		GO_GOLANGCI_VERSION="$${GO_GOMOD_VERSION}"; \
	fi; \
	if [ "$${GO_GOMOD_VERSION}" != "$${GO_GOLANGCI_VERSION}" ]; then \
		echo "ERROR: go.mod uses Go $$GO_GOMOD_VERSION but .golangci.yml uses $$GO_GOLANGCI_VERSION"; \
		echo "Please update '.golangci.yml:run.go' to $$GO_GOMOD_VERSION (major.minor) and rerun 'make precommit'."; \
		exit 1; \
	fi


# This runs all necessary steps to prepare for a commit.
precommit: ensure-go-version-upgrade sync-deps sync-img-env vet tidy go-lint py-fmt py-lint generate manifests uv-lock generate-quick-install-scripts

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
	KUBEBUILDER_ASSETS="$$($(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" go test --timeout 20m $$(go list ./pkg/...) ./cmd/... -coverprofile coverage.out -coverpkg ./pkg/... ./cmd...

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
	# Given that llmisvc CRs and CRDs are packaged together, when using kustomize build a race condition will occur.
	# This is because before the CRD is registered to the api server, kustomize will attempt to create the CR.
	# The below kubectl apply and kubectl wait commands are necessary to avoid this race condition.
	kubectl apply --server-side=true --force-conflicts -k config/crd/full
	kubectl apply --server-side=true --force-conflicts -k config/crd/full/localmodel
	kubectl apply --server-side=true --force-conflicts -k config/crd/full/llmisvc
	kubectl wait --for=condition=established --timeout=60s crd/llminferenceserviceconfigs.serving.kserve.io
	# Remove the certmanager certificate if KSERVE_ENABLE_SELF_SIGNED_CA is not false
	cd config/default && if [ ${KSERVE_ENABLE_SELF_SIGNED_CA} != false ]; then \
	echo > ../certmanager/certificate.yaml; \
	echo > ../certmanager/llmisvc/certificate.yaml; \
	else git checkout HEAD -- ../certmanager/certificate.yaml ../certmanager/llmisvc/certificate.yaml; fi;
	kubectl apply --server-side=true -k config/default
	if [ ${KSERVE_ENABLE_SELF_SIGNED_CA} != false ]; then \
		./hack/self-signed-ca.sh; \
		./hack/self-signed-ca.sh --service llmisvc-webhook-server-service \
			--secret llmisvc-webhook-server-cert \
			--webhookDeployment llmisvc-controller-manager \
			--validatingWebhookName llminferenceservice.serving.kserve.io \
			--validatingWebhookName llminferenceserviceconfig.serving.kserve.io; \
	fi;
	kubectl wait --for=condition=ready pod -l control-plane=kserve-controller-manager -n kserve --timeout=300s
	kubectl wait --for=condition=ready pod -l control-plane=llmisvc-controller-manager -n kserve --timeout=300s
	kubectl apply  --server-side=true  -k config/clusterresources
	git checkout HEAD -- config/certmanager/certificate.yaml config/certmanager/llmisvc/certificate.yaml


deploy-dev: manifests
	# Given that llmisvc CRs and CRDs are packaged together, when using kustomize build a race condition will occur.
	# This is because before the CRD is registered to the api server, kustomize will attempt to create the CR.
	# The below kubectl apply and kubectl wait commands are necessary to avoid this race condition.
	kubectl apply --server-side=true --force-conflicts -k config/crd/full
	kubectl apply --server-side=true --force-conflicts -k config/crd/full/localmodel
	kubectl apply --server-side=true --force-conflicts -k config/crd/full/llmisvc
	kubectl wait --for=condition=established --timeout=60s crd/llminferenceserviceconfigs.serving.kserve.io
	./hack/image_patch_dev.sh development
	
	@echo "Deploy KServe,LocalModel and LLMInferenceService"
	hack/setup/infra/manage.cert-manager-helm.sh
	KSERVE_OVERLAY_DIR=development hack/setup/infra/manage.kserve-kustomize.sh
	
	@echo "Create ClusterServingRuntimes as part of default deployment"
	kubectl wait --for=condition=ready pod -l control-plane=kserve-controller-manager -n kserve --timeout=300s
	kubectl wait --for=condition=ready pod -l control-plane=llmisvc-controller-manager -n kserve --timeout=300s
	kubectl apply --server-side=true --force-conflicts -k config/clusterresources

# Quick redeploy after code changes (rebuild images and update deployments)
redeploy-dev-image:
	./hack/image_patch_dev.sh development
	kubectl apply --server-side=true --force-conflicts -k config/overlays/development
	
	kubectl rollout restart deployment/kserve-controller-manager -n kserve
	kubectl rollout status deployment/kserve-controller-manager -n kserve --timeout=300s
	
	kubectl rollout restart deployment/llmisvc-controller-manager -n kserve
	kubectl rollout status deployment/llmisvc-controller-manager -n kserve --timeout=300s
	
	@echo "Deployments updated successfully"
	kubectl get pods -n kserve

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

deploy-dev-predictive: docker-push-predictive
	./hack/serving_runtime_image_patch.sh "kserve-predictiveserver.yaml" "${KO_DOCKER_REPO}/${PREDICTIVE_IMG}"

deploy-dev-huggingface: docker-push-huggingface
	./hack/serving_runtime_image_patch.sh "kserve-huggingfaceserver.yaml" "${KO_DOCKER_REPO}/${HUGGINGFACE_IMG}"

deploy-dev-storageInitializer: docker-push-storageInitializer
	./hack/storageInitializer_patch_dev.sh ${KO_DOCKER_REPO}/${STORAGE_INIT_IMG}
	kubectl apply --server-side=true -k config/overlays/dev-image-config
	
deploy-helm:
	USE_LOCAL_CHARTS=true ./hack/setup/infra/manage.kserve-helm.sh

undeploy:
	kubectl delete -k config/default

undeploy-dev:
	kubectl delete -k config/overlays/development

bump-version:
	@echo "bumping version numbers for this release"
	@hack/prepare-for-release.sh $(PRIOR_VERSION) $(NEW_VERSION)

# Build the docker image
docker-build:
	${ENGINE} buildx build ${ARCH} . -t ${KO_DOCKER_REPO}/${CONTROLLER_IMG}
	@echo "updating kustomize image patch file for manager resource"

	# Use perl instead of sed to avoid OSX/Linux compatibility issue:
	# https://stackoverflow.com/questions/34533893/sed-command-creating-unwanted-duplicates-of-file-with-e-extension
	perl -pi -e 's@image: .*@image: '"${CONTROLLER_IMG}"'@' ./config/default/manager_image_patch.yaml

# Push the docker image
docker-push:
	docker push ${KO_DOCKER_REPO}/${CONTROLLER_IMG}

docker-build-llmisvc:
	${ENGINE} buildx build ${ARCH} -t ${KO_DOCKER_REPO}/${LLMISVC_CONTROLLER_IMG} -f llmisvc-controller.Dockerfile .

docker-push-llmisvc: docker-build-llmisvc
	${ENGINE} buildx build ${ARCH} --push -t ${KO_DOCKER_REPO}/${LLMISVC_CONTROLLER_IMG} -f llmisvc-controller.Dockerfile .

docker-build-localmodel:
	${ENGINE} buildx build ${ARCH} -t ${KO_DOCKER_REPO}/${LOCALMODEL_CONTROLLER_IMG} -f localmodel.Dockerfile .

docker-push-localmodel: docker-build-localmodel
	${ENGINE} buildx build ${ARCH} --push -t ${KO_DOCKER_REPO}/${LOCALMODEL_CONTROLLER_IMG} -f localmodel.Dockerfile .

docker-build-localmodelnode-agent:
	${ENGINE} buildx build ${ARCH} -t ${KO_DOCKER_REPO}/${LOCALMODEL_AGENT_IMG} -f localmodel-agent.Dockerfile .

docker-push-localmodelnode-agent: docker-build-localmodelnode-agent
	${ENGINE} buildx build ${ARCH} --push -t ${KO_DOCKER_REPO}/${LOCALMODEL_AGENT_IMG} -f localmodel-agent.Dockerfile .

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

docker-build-predictive:
	cd python && ${ENGINE} buildx build ${ARCH} --build-arg BASE_IMAGE=${BASE_IMG} -t ${KO_DOCKER_REPO}/${PREDICTIVE_IMG} -f predictiveserver.Dockerfile .

docker-push-predictive: docker-build-predictive
	cd python && ${ENGINE} buildx build ${ARCH} --push --build-arg BASE_IMAGE=${BASE_IMG} -t ${KO_DOCKER_REPO}/${PREDICTIVE_IMG} -f predictiveserver.Dockerfile .

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
	cd python && ${ENGINE} buildx build ${ARCH} -t ${KO_DOCKER_REPO}/${IMAGE_TRANSFORMER_IMG} -f custom_transformer.Dockerfile .

docker-push-custom-transformer: docker-build-custom-transformer
	${ENGINE} push ${KO_DOCKER_REPO}/${IMAGE_TRANSFORMER_IMG}

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
	cd python && ${ENGINE} buildx build ${ARCH} -t ${KO_DOCKER_REPO}/${HUGGINGFACE_IMG} -f huggingface_server.Dockerfile .

docker-push-huggingface: docker-build-huggingface
	${ENGINE} push ${KO_DOCKER_REPO}/${HUGGINGFACE_IMG}

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

