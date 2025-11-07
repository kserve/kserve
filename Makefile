# The Go and Python based tools are defined in Makefile.tools.mk.
include Makefile.tools.mk

# Load dependency versions
include kserve-deps.env

# Base Image URL
BASE_IMG ?= python:3.11-slim-bookworm
PMML_BASE_IMG ?= eclipse-temurin:21-jdk-noble

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
LLMISVC_IMG ?= kserve-llmisvc-controller:latest

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
	@hack/setup/scripts/generate-versions-from-gomod.sh

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
	
	# Move LLMISVC CRD to llmisvc folder
	                   
	mv config/crd/full/serving.kserve.io_llminferenceservices.yaml config/crd/full/llmisvc/serving.kserve.io_llminferenceservices.yaml
	mv config/crd/full/serving.kserve.io_llminferenceserviceconfigs.yaml config/crd/full/llmisvc/serving.kserve.io_llminferenceserviceconfigs.yaml
	
	# Ensure Helm chart directories exist before copying files
	@mkdir -p charts/kserve-resources/templates/localmodel
	@mkdir -p charts/kserve-resources/templates/localmodelnode
	@mkdir -p charts/kserve-llmisvc-crd/templates
	@mkdir -p charts/kserve-crd/templates
	@mkdir -p charts/kserve-llmisvc-resources/templates
	@mkdir -p charts/kserve-resources/templates
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

	# Remove validation for the LLMInferenceServiceConfig API so that we can use Go templates to inject values at runtime.
	@$(YQ) 'del(.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.router.properties.route.properties.http.properties.spec.properties.rules.items.properties.matches.items.properties.path.x-kubernetes-validations)' -i config/crd/full/llmisvc/serving.kserve.io_llminferenceserviceconfigs.yaml
	@$(YQ) 'del(.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.router.properties.route.properties.http.properties.spec.properties.rules.items.properties.filters.items.properties.urlRewrite.properties.path.x-kubernetes-validations)' -i config/crd/full/llmisvc/serving.kserve.io_llminferenceserviceconfigs.yaml
	@$(YQ) 'del(.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.router.properties.route.properties.http.properties.spec.properties.parentRefs.items.properties.namespace.pattern)' -i config/crd/full/llmisvc/serving.kserve.io_llminferenceserviceconfigs.yaml
	# Remove validation for the LLMInferenceServiceConfig API so that we can override only specific values.
	@$(YQ) 'del(.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.worker.required)' -i config/crd/full/llmisvc/serving.kserve.io_llminferenceserviceconfigs.yaml
	@$(YQ) 'del(.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.template.required)' -i config/crd/full/llmisvc/serving.kserve.io_llminferenceserviceconfigs.yaml
	@$(YQ) 'del(.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.prefill.properties.template.required)' -i config/crd/full/llmisvc/serving.kserve.io_llminferenceserviceconfigs.yaml
	@$(YQ) 'del(.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.prefill.properties.worker.required)' -i config/crd/full/llmisvc/serving.kserve.io_llminferenceserviceconfigs.yaml
	@$(YQ) 'del(.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.router.properties.scheduler.properties.template.required)' -i config/crd/full/llmisvc/serving.kserve.io_llminferenceserviceconfigs.yaml
	# Remove validation for the LLMInferenceService API so that we can override only specific values.
	@$(YQ) 'del(.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.worker.required)' -i config/crd/full/llmisvc/serving.kserve.io_llminferenceservices.yaml
	@$(YQ) 'del(.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.template.required)' -i config/crd/full/llmisvc/serving.kserve.io_llminferenceservices.yaml
	@$(YQ) 'del(.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.prefill.properties.template.required)' -i config/crd/full/llmisvc/serving.kserve.io_llminferenceservices.yaml
	@$(YQ) 'del(.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.prefill.properties.worker.required)' -i config/crd/full/llmisvc/serving.kserve.io_llminferenceservices.yaml
	@$(YQ) 'del(.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.router.properties.scheduler.properties.template.required)' -i config/crd/full/llmisvc/serving.kserve.io_llminferenceservices.yaml

	# DO NOT COPY to helm chart. It needs to be created before the Envoy Gateway or you will need to restart the Envoy Gateway controller.
	# The llmisvc helm chart needs to be installed after the Envoy Gateway as well, so it needs to be created before the llmisvc helm chart.
	# Only fetch if file doesn't exist or is empty (avoid network timeout during precommit)
	@if [ ! -s config/llmisvc/gateway-inference-extension.yaml ]; then \
		echo "Fetching gateway-inference-extension CRD..."; \
		kubectl kustomize https://github.com/kubernetes-sigs/gateway-api-inference-extension.git/config/crd?ref=$(GIE_VERSION) > config/llmisvc/gateway-inference-extension.yaml; \
	else \
		echo "gateway-inference-extension.yaml already exists, skipping fetch"; \
	fi
	@cp config/llmisvc/gateway-inference-extension.yaml test/crds/gateway-inference-extension.yaml

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
	# Copy the minimal crd to the helm chart
	cp config/crd/minimal/*.yaml charts/kserve-crd-minimal/templates/
	cp config/crd/minimal/llmisvc/*.yaml charts/kserve-llmisvc-crd-minimal/templates/
	rm charts/kserve-crd-minimal/templates/kustomization.yaml
	rm charts/kserve-llmisvc-crd-minimal/templates/kustomization.yaml
	# Generate llmisvc rbac
	@$(CONTROLLER_GEN) rbac:roleName=llmisvc-manager-role paths={./pkg/controller/v1alpha1/llmisvc} output:rbac:artifacts:config=config/rbac/llmisvc
	# Note: RBAC Helm templates are now generated via helm-generate-llmisvc target (includes bindings)
	# Copy llmisvc crd to kserve-llmisvc-crd chart
	cp config/crd/full/llmisvc/serving.kserve.io_llminferenceservices.yaml charts/kserve-llmisvc-crd/templates/
	cp config/crd/full/llmisvc/serving.kserve.io_llminferenceserviceconfigs.yaml charts/kserve-llmisvc-crd/templates/
	# Copy llmisvc crd to kserve-crd chart (for combined deployments)
	cp config/crd/full/llmisvc/serving.kserve.io_llminferenceservices.yaml charts/kserve-crd/templates/
	cp config/crd/full/llmisvc/serving.kserve.io_llminferenceserviceconfigs.yaml charts/kserve-crd/templates/

.PHONY: helm-generate-llmisvc
helm-generate-llmisvc: helmify yq
	@echo "=========================================="
	@echo "Generating LLMISvc Helm chart (standalone, 100% automated)"
	@echo "=========================================="

	# Generate standalone LLMISvc from overlay (excludes CRDs - they're in kserve-llmisvc-crd chart)
	@echo "Generating standalone LLMISvc templates from Kustomize overlay..."
	@mkdir -p build-helm
	@rm -rf build-helm/llmisvc-chart
	@kubectl kustomize config/overlays/llmisvc > build-helm/llmisvc.yaml
	# Filter out CRDs (they're managed by kserve-llmisvc-crd chart)
	@$(YQ) eval 'select(.kind != "CustomResourceDefinition")' build-helm/llmisvc.yaml > build-helm/llmisvc-no-crds.yaml
	@cat build-helm/llmisvc-no-crds.yaml | $(HELMIFY) build-helm/llmisvc-chart

	# Remove CRDs from generated chart (they're managed by kserve-llmisvc-crd chart)
	@echo "Removing CRDs from chart templates..."
	@rm -f build-helm/llmisvc-chart/templates/*-crd.yaml

	# Escape embedded Go templates
	@echo "Escaping KServe-specific Go templates..."
	@./hack/escape_helm_templates.py build-helm/llmisvc-chart/templates/*.yaml

	# Fix chart references
	@echo "Fixing chart references..."
	@for file in build-helm/llmisvc-chart/templates/*.yaml build-helm/llmisvc-chart/templates/_helpers.tpl; do \
		if [ -f "$$file" ]; then \
			sed -i 's/llmisvc-chart\./llm-isvc-resources./g' "$$file"; \
		fi; \
	done

	# Copy EVERYTHING to actual chart (100% automated)
	@echo "Copying all generated templates and values..."
	@mkdir -p charts/kserve-llmisvc-resources/templates
	@cp -r build-helm/llmisvc-chart/templates/* charts/kserve-llmisvc-resources/templates/
	# Copy config-llm files directly from Kustomize source (preserves initContainers)
	@echo "Copying config-llm files directly from Kustomize source..."
	@for file in config/llmisvcconfig/config-llm-*.yaml; do \
		if [ -f "$$file" ]; then \
			cp "$$file" charts/kserve-llmisvc-resources/templates/kserve-$$(basename "$$file"); \
		fi; \
	done
	# Escape Go templates in config-llm files (they contain KServe runtime templates)
	@echo "Escaping Go templates in config-llm files..."
	@./hack/escape_helm_templates.py charts/kserve-llmisvc-resources/templates/kserve-config-llm-*.yaml
	# Fix helmify output: ensure {{- if }} syntax is correct (helmify sometimes generates {{ if instead of {{- if)
	@for file in charts/kserve-llmisvc-resources/templates/*.yaml; do \
		if [ -f "$$file" ]; then \
			sed -i 's/{{ if /{{- if /g' "$$file"; \
		fi; \
	done
	@mkdir -p charts/kserve-llmisvc-resources
	@cp build-helm/llmisvc-chart/values.yaml charts/kserve-llmisvc-resources/values.yaml
	# Copy Chart.yaml if it doesn't exist, or preserve existing one
	@if [ ! -f charts/kserve-llmisvc-resources/Chart.yaml ]; then \
		cp build-helm/llmisvc-chart/Chart.yaml charts/kserve-llmisvc-resources/Chart.yaml; \
		sed -i 's/name: llmisvc-chart/name: kserve-llmisvc-resources/g' charts/kserve-llmisvc-resources/Chart.yaml; \
	fi
	@echo "Note: Chart.yaml is preserved (contains version and metadata)"

	# Fix hardcoded resource names that controllers expect
	@echo "Fixing hardcoded resource names..."
	@sed -i "s/name: {{ include \"llm-isvc-resources\.fullname\" \. }}-inferenceservice-config/name: inferenceservice-config/g" charts/kserve-llmisvc-resources/templates/inferenceservice-config.yaml || true
	# Fix deployment name to match Kustomize name (extracted from config files)
	@if [ -f charts/kserve-llmisvc-resources/templates/deployment.yaml ]; then \
		DEPLOYMENT_NAME=$$($(YQ) eval 'select(.kind == "Deployment") | .metadata.name' config/llmisvc/manager.yaml 2>/dev/null || echo ""); \
		if [ -n "$$DEPLOYMENT_NAME" ]; then \
			sed -i "s|name: {{ include \"llm-isvc-resources\.fullname\" \. }}-$$DEPLOYMENT_NAME|name: $$DEPLOYMENT_NAME|g" charts/kserve-llmisvc-resources/templates/deployment.yaml || true; \
		fi; \
	fi
	
	# Fix JSON field rendering (remove toYaml for JSON strings - they're already strings, use |- for multi-line strings)
	@echo "Fixing ConfigMap data field rendering..."
	@if [ -f charts/kserve-llmisvc-resources/templates/inferenceservice-config.yaml ]; then \
		python3 hack/fix_inferenceservice_config_template.py charts/kserve-llmisvc-resources/templates/inferenceservice-config.yaml; \
	fi
	
	# Fix LLMInferenceServiceConfig names (remove Helm prefix - controllers expect original names)
	@echo "Fixing LLMInferenceServiceConfig resource names..."
	@for file in charts/kserve-llmisvc-resources/templates/kserve-config-llm-*.yaml; do \
		if [ -f "$$file" ]; then \
			sed -i "s/name: {{ include \"llm-isvc-resources\.fullname\" \. }}-kserve-config-llm-/name: kserve-config-llm-/g" "$$file"; \
		fi; \
	done


	# Make inferenceservice-config conditional (only create if KServe doesn't already manage it)
	@echo "Making inferenceservice-config conditional..."
	@if [ -f charts/kserve-llmisvc-resources/templates/inferenceservice-config.yaml ]; then \
		if ! grep -q "createInferenceServiceConfig" charts/kserve-llmisvc-resources/templates/inferenceservice-config.yaml; then \
			sed -i '1s/^/{{- if and .Values.createInferenceServiceConfig (not (lookup "v1" "ConfigMap" "kserve" "inferenceservice-config")) }}\n/' charts/kserve-llmisvc-resources/templates/inferenceservice-config.yaml; \
			echo '{{- end }}' >> charts/kserve-llmisvc-resources/templates/inferenceservice-config.yaml; \
		fi; \
	fi

	# Ensure createInferenceServiceConfig value exists in values.yaml
	@if ! grep -q "^createInferenceServiceConfig:" charts/kserve-llmisvc-resources/values.yaml; then \
		sed -i '1i# Whether to create the inferenceservice-config ConfigMap\n# Set to false if KServe is already installed (KServe creates this ConfigMap)\ncreateInferenceServiceConfig: true\n' charts/kserve-llmisvc-resources/values.yaml; \
	fi


	# Validate
	@echo "Validating Helm chart..."
	@helm lint charts/kserve-llmisvc-resources
	@helm template test charts/kserve-llmisvc-resources --dry-run > /dev/null

	@echo "✅ LLMISvc Helm chart fully generated (100% automated, 0% manual)"
	@echo "   Output: charts/kserve-llmisvc-resources/"

.PHONY: helm-generate-kserve
helm-generate-kserve: helmify yq
	@echo "=========================================="
	@echo "Generating KServe Helm chart (includes LLMISvc, 100% automated)"
	@echo "=========================================="

	# Generate combined KServe + LLMISvc from config/default (excludes CRDs - they're in kserve-crd chart)
	@echo "Generating combined KServe+LLMISvc templates from config/default..."
	@mkdir -p build-helm
	@rm -rf build-helm/kserve-chart
	@kubectl kustomize config/default > build-helm/kserve-all.yaml
	# Filter out CRDs (they're managed by kserve-crd/kserve-crd-minimal charts)
	@$(YQ) eval 'select(.kind != "CustomResourceDefinition")' build-helm/kserve-all.yaml > build-helm/kserve-all-no-crds.yaml
	@cat build-helm/kserve-all-no-crds.yaml | $(HELMIFY) build-helm/kserve-chart

	# Escape embedded Go templates (for LLMISvc ConfigMaps)
	@echo "Escaping KServe-specific Go templates..."
	@./hack/escape_helm_templates.py build-helm/kserve-chart/templates/*.yaml

	# Fix chart references
	@echo "Fixing chart references..."
	@for file in build-helm/kserve-chart/templates/*.yaml build-helm/kserve-chart/templates/_helpers.tpl; do \
		if [ -f "$$file" ]; then \
			sed -i 's/kserve-chart\./kserve-resources./g' "$$file"; \
		fi; \
	done

	# Copy EVERYTHING to actual chart (100% automated)
	@echo "Copying all generated templates and values..."
	@mkdir -p charts/kserve-resources/templates/localmodel
	@mkdir -p charts/kserve-resources/templates/localmodelnode
	@cp -r build-helm/kserve-chart/templates/* charts/kserve-resources/templates/
	# Copy config-llm files directly from Kustomize source (preserves initContainers)
	@echo "Copying config-llm files directly from Kustomize source..."
	@for file in config/llmisvcconfig/config-llm-*.yaml; do \
		if [ -f "$$file" ]; then \
			cp "$$file" charts/kserve-resources/templates/kserve-$$(basename "$$file"); \
		fi; \
	done
	# Escape Go templates in config-llm files (they contain KServe runtime templates)
	@echo "Escaping Go templates in config-llm files..."
	@./hack/escape_helm_templates.py charts/kserve-resources/templates/kserve-config-llm-*.yaml
	# Fix helmify output: ensure {{- if }} syntax is correct (helmify sometimes generates {{ if instead of {{- if)
	@for file in charts/kserve-resources/templates/*.yaml; do \
		if [ -f "$$file" ]; then \
			sed -i 's/{{ if /{{- if /g' "$$file"; \
		fi; \
	done
	@mkdir -p charts/kserve-resources
	@cp build-helm/kserve-chart/values.yaml charts/kserve-resources/values.yaml
	# Copy Chart.yaml if it doesn't exist, or preserve existing one
	@if [ ! -f charts/kserve-resources/Chart.yaml ]; then \
		cp build-helm/kserve-chart/Chart.yaml charts/kserve-resources/Chart.yaml; \
		sed -i 's/name: kserve-chart/name: kserve-resources/g' charts/kserve-resources/Chart.yaml; \
	fi
	@echo "Note: Chart.yaml is preserved (contains version and metadata)"

	# Remove CRDs from generated chart (they're managed by kserve-crd/kserve-crd-minimal charts)
	@echo "Removing CRDs from chart templates..."
	@rm -f charts/kserve-resources/templates/*-crd.yaml

	# Fix hardcoded resource names that controllers expect
	@echo "Fixing hardcoded resource names..."
	@sed -i "s/name: {{ include \"kserve-resources\.fullname\" \. }}-inferenceservice-config/name: inferenceservice-config/g" charts/kserve-resources/templates/inferenceservice-config.yaml
	# Fix deployment names to match Kustomize names (extracted from config files)
	@if [ -f charts/kserve-resources/templates/deployment.yaml ]; then \
		for config_file in config/manager/manager.yaml config/llmisvc/manager.yaml config/localmodels/manager.yaml; do \
			if [ -f "$$config_file" ]; then \
				DEPLOYMENT_NAME=$$($(YQ) eval 'select(.kind == "Deployment") | .metadata.name' "$$config_file" 2>/dev/null || echo ""); \
				if [ -n "$$DEPLOYMENT_NAME" ]; then \
					sed -i "s|name: {{ include \"kserve-resources\.fullname\" \. }}-$$DEPLOYMENT_NAME|name: $$DEPLOYMENT_NAME|g" charts/kserve-resources/templates/deployment.yaml || true; \
				fi; \
			fi; \
		done; \
	fi
	# Fix service names to match Kustomize names (extracted from config files)
	@if [ -f charts/kserve-resources/templates/kserve-controller-manager-service.yaml ]; then \
		SERVICE_NAME=$$($(YQ) eval 'select(.kind == "Service") | .metadata.name' config/manager/service.yaml 2>/dev/null || echo ""); \
		if [ -n "$$SERVICE_NAME" ]; then \
			sed -i "s|name: {{ include \"kserve-resources\.fullname\" \. }}-$$SERVICE_NAME|name: $$SERVICE_NAME|g" charts/kserve-resources/templates/kserve-controller-manager-service.yaml || true; \
		fi; \
	fi
	@if [ -f charts/kserve-resources/templates/kserve-controller-manager-metrics-service.yaml ]; then \
		METRICS_SERVICE_NAME=$$($(YQ) eval 'select(.kind == "Service") | .metadata.name' config/rbac/auth_proxy_service.yaml 2>/dev/null || echo ""); \
		if [ -n "$$METRICS_SERVICE_NAME" ]; then \
			sed -i "s|name: {{ include \"kserve-resources\.fullname\" \. }}-$$METRICS_SERVICE_NAME|name: $$METRICS_SERVICE_NAME|g" charts/kserve-resources/templates/kserve-controller-manager-metrics-service.yaml || true; \
		fi; \
	fi
	# Fix service account names to match Kustomize names (extracted from config files)
	# Note: Service account names are already hardcoded in the template, but this ensures they stay correct after regeneration
	@if [ -f charts/kserve-resources/templates/serviceaccount.yaml ]; then \
		SA_NAMES=""; \
		for sa_file in config/rbac/service_account.yaml config/rbac/llmisvc/service_account.yaml config/rbac/localmodel/service_account.yaml config/rbac/localmodelnode/service_account.yaml; do \
			if [ -f "$$sa_file" ]; then \
				SA_NAME=$$($(YQ) eval 'select(.kind == "ServiceAccount") | .metadata.name' "$$sa_file" 2>/dev/null || echo ""); \
				if [ -n "$$SA_NAME" ]; then \
					SA_NAMES="$$SA_NAMES $$SA_NAME"; \
					sed -i "0,/name: {{ include \"kserve-resources\.serviceAccountName\" \. }}/s/name: {{ include \"kserve-resources\.serviceAccountName\" \. }}/name: $$SA_NAME/" charts/kserve-resources/templates/serviceaccount.yaml || true; \
				fi; \
			fi; \
		done; \
	fi
	
	# Fix JSON field rendering (remove toYaml for JSON strings - they're already strings, use |- for multi-line strings)
	@echo "Fixing ConfigMap data field rendering..."
	@if [ -f charts/kserve-resources/templates/inferenceservice-config.yaml ]; then \
		python3 hack/fix_inferenceservice_config_template.py charts/kserve-resources/templates/inferenceservice-config.yaml; \
	fi
	
	# Fix LLMInferenceServiceConfig names (remove Helm prefix - controllers expect original names)
	@echo "Fixing LLMInferenceServiceConfig resource names..."
	@for file in charts/kserve-resources/templates/kserve-config-llm-*.yaml; do \
		if [ -f "$$file" ]; then \
			sed -i "s/name: {{ include \"kserve-resources\.fullname\" \. }}-kserve-config-llm-/name: kserve-config-llm-/g" "$$file"; \
		fi; \
	done

	# Add required values for conditional templates
	@echo "Adding required Helm values..."
	@echo "" >> charts/kserve-resources/values.yaml
	@echo "# Local model configuration" >> charts/kserve-resources/values.yaml
	@echo "kserve:" >> charts/kserve-resources/values.yaml
	@echo "  localmodel:" >> charts/kserve-resources/values.yaml
	@echo "    enabled: false" >> charts/kserve-resources/values.yaml

	# Fix malformed Certificate dnsNames
	@echo "Fixing Certificate templates..."
	@./hack/fix_certificate_dnsnames.py

	# Validate
	@echo "Validating Helm chart..."
	@helm lint charts/kserve-resources
	@helm template test charts/kserve-resources --dry-run > /dev/null

	@echo "✅ KServe Helm chart fully generated (includes LLMISvc, 100% automated, 0% manual)"
	@echo "   Output: charts/kserve-resources/"
    # Copy Test inferenceconfig configmap to test overlay
	cp config/configmap/inferenceservice.yaml config/overlays/test/configmap/inferenceservice.yaml

# Generate code
generate: controller-gen helm-docs manifests
	hack/update-codegen.sh
	hack/update-openapigen.sh
	hack/python-sdk/client-gen.sh
	$(HELM_DOCS) --chart-search-root=charts --output-file=README.md
	# Generate Helm charts from Kustomize configs
	@echo "Generating Helm charts..."
	@$(MAKE) helm-generate-llmisvc
	@$(MAKE) helm-generate-kserve

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


# This runs all necessary steps to prepare for a commit.
precommit: sync-deps vet tidy go-lint py-fmt py-lint generate manifests uv-lock

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
	kubectl apply --server-side=true --force-conflicts -k config/crd
	kubectl wait --for=condition=established --timeout=60s crd/llminferenceserviceconfigs.serving.kserve.io
	# Remove the certmanager certificate if KSERVE_ENABLE_SELF_SIGNED_CA is not false
	cd config/default && if [ ${KSERVE_ENABLE_SELF_SIGNED_CA} != false ]; then \
	echo > ../certmanager/certificate.yaml; \
	else git checkout HEAD -- ../certmanager/certificate.yaml; fi;
	kubectl apply --server-side=true -k config/default
	if [ ${KSERVE_ENABLE_SELF_SIGNED_CA} != false ]; then ./hack/self-signed-ca.sh; fi;
	kubectl wait --for=condition=ready pod -l control-plane=kserve-controller-manager -n kserve --timeout=300s
	kubectl wait --for=condition=ready pod -l control-plane=llmisvc-controller-manager -n kserve --timeout=300s
	kubectl apply  --server-side=true  -k config/clusterresources
	git checkout HEAD -- config/certmanager/certificate.yaml


deploy-dev: manifests
	# Given that llmisvc CRs and CRDs are packaged together, when using kustomize build a race condition will occur.
	# This is because before the CRD is registered to the api server, kustomize will attempt to create the CR.
	# The below kubectl apply and kubectl wait commands are necessary to avoid this race condition.
	kubectl apply --server-side=true --force-conflicts -k config/crd
	kubectl wait --for=condition=established --timeout=60s crd/llminferenceserviceconfigs.serving.kserve.io
	./hack/image_patch_dev.sh development
	# Remove the certmanager certificate if KSERVE_ENABLE_SELF_SIGNED_CA is not false
	cd config/default && if [ ${KSERVE_ENABLE_SELF_SIGNED_CA} != false ]; then \
	echo > ../certmanager/certificate.yaml; \
	else git checkout HEAD -- ../certmanager/certificate.yaml; fi;
	kubectl apply --server-side=true --force-conflicts -k config/overlays/development
	if [ ${KSERVE_ENABLE_SELF_SIGNED_CA} != false ]; then ./hack/self-signed-ca.sh; fi;
	# TODO: Add runtimes as part of default deployment
	kubectl wait --for=condition=ready pod -l control-plane=kserve-controller-manager -n kserve --timeout=300s
	kubectl wait --for=condition=ready pod -l control-plane=llmisvc-controller-manager -n kserve --timeout=300s
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
	
deploy-helm: manifests helm-generate-kserve
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
	${ENGINE} buildx build ${ARCH} . -t ${KO_DOCKER_REPO}/${IMG}
	@echo "updating kustomize image patch file for manager resource"

	# Use perl instead of sed to avoid OSX/Linux compatibility issue:
	# https://stackoverflow.com/questions/34533893/sed-command-creating-unwanted-duplicates-of-file-with-e-extension
	perl -pi -e 's@image: .*@image: '"${IMG}"'@' ./config/default/manager_image_patch.yaml

# Push the docker image
docker-push:
	docker push ${KO_DOCKER_REPO}/${IMG}

docker-build-llmisvc:
	${ENGINE} buildx build ${ARCH} -t ${KO_DOCKER_REPO}/${LLMISVC_IMG} -f llmisvc-controller.Dockerfile .

docker-push-llmisvc: docker-build-llmisvc
	${ENGINE} buildx build ${ARCH} --push -t ${KO_DOCKER_REPO}/${LLMISVC_IMG} -f llmisvc-controller.Dockerfile .

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

