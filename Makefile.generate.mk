# Code/manifest generation and artifact sync.

##@ Code generation

# --fail keeps an HTTP error body out of the downloaded artifact; 5xx and timeouts are
# retried so a flaky release download does not fail the whole generation run.
CURL ?= curl --fail --silent --show-error --location --retry 3 --retry-delay 2

.PHONY: sync-deps
sync-deps:
	@@python3 hack/setup/scripts/generate-versions-from-gomod.py --no-cache

.PHONY: sync-img-env
sync-img-env:
	@python3 hack/setup/scripts/generate-images-sh.py

generate-quick-install-scripts: validate-infra-scripts $(PYTHON_VENV)
	@$(PYTHON_BIN)/pip install -q -r hack/setup/scripts/install-script-generator/requirements.txt
	@$(PYTHON_BIN)/python hack/setup/scripts/install-script-generator/generator.py

generate-chart-manifests: ## After config/ or charts/_common edits: regenerate chart files, then helm lint. Needs helm.
	@bash hack/setup/scripts/generate_chart_manifests.sh
	make lint-helm-charts
	make verify-helm-helpers-consistency

manifests: controller-gen kustomize yq ## After API type or RBAC marker changes: CRDs, RBAC, deepcopy. Needs network.
	@$(CONTROLLER_GEN) $(CRD_OPTIONS) paths=./pkg/apis/serving/... output:crd:dir=config/crd/full	
	@$(CONTROLLER_GEN) rbac:roleName=kserve-manager-role paths={./pkg/controller/v1alpha1/inferencegraph,./pkg/controller/v1alpha1/trainedmodel,./pkg/controller/v1beta1/inferenceservice} output:rbac:artifacts:config=config/rbac
	@$(CONTROLLER_GEN) rbac:roleName=kserve-llmisvc-manager-role paths=./pkg/controller/v1alpha2/llmisvc output:rbac:artifacts:config=config/rbac/llmisvc
	@$(CONTROLLER_GEN) rbac:roleName=kserve-localmodel-manager-role paths=./pkg/controller/v1alpha1/localmodel output:rbac:artifacts:config=config/rbac/localmodel
	@$(CONTROLLER_GEN) rbac:roleName=kserve-kernelcache-nodegroup-manager paths=./pkg/controller/v1alpha1/kernelcache output:rbac:artifacts:config=config/rbac/kernelcache
	@$(CONTROLLER_GEN) rbac:roleName=kserve-localmodelnode-agent-role paths=./pkg/controller/v1alpha1/localmodelnode output:rbac:artifacts:config=config/rbac/localmodelnode
	@$(CONTROLLER_GEN) rbac:roleName=kserve-kernelcachenode-agent-role paths=./pkg/controller/v1alpha1/kernelcachenode output:rbac:artifacts:config=config/rbac/kernelcachenode
	# Hook for distro-specific manifest generation (override via Makefile.overrides.mk).
	@$(MAKE) manifests-distro

	# DO NOT COPY to helm chart. It needs to be created before the Envoy Gateway or you will need to restart the Envoy Gateway controller.
	# The llmisvc helm chart needs to be installed after the Envoy Gateway as well, so it needs to be created before the llmisvc helm chart.
	# Pull upstream GIE v1 CRDs (InferencePool, etc.) from release artifact, then append the
	# llm-d.ai CRDs (InferenceObjective, InferenceModelRewrite) from the llm-d-router release.
	# Assembled in a temp file and installed only once both downloads succeed: curl writes the
	# HTTP error body to its output stream, so a transient failure would otherwise leave an
	# unparseable manifest in the tree and break every kustomize build that reads it.
	@set -e; tmp=$$(mktemp); trap 'rm -f "$$tmp"' EXIT; \
		$(CURL) https://github.com/kubernetes-sigs/gateway-api-inference-extension/releases/download/$(GIE_VERSION)/v1-manifests.yaml > "$$tmp"; \
		echo "---" >> "$$tmp"; \
		$(CURL) https://github.com/llm-d/llm-d-router/releases/download/$(LLMD_ROUTER_VERSION)/manifests.yaml >> "$$tmp"; \
		install -m 0644 "$$tmp" config/llmisvc/gateway-inference-extension.yaml
	# Workaround to update main-dev version from llm-d-router release as annotation
	sed -i 's|llm-d.ai/bundle-version: main-dev|llm-d.ai/bundle-version: $(LLMD_ROUTER_VERSION)|' config/llmisvc/gateway-inference-extension.yaml
	cp config/llmisvc/gateway-inference-extension.yaml test/crds/gateway-inference-extension.yaml
	cat test/crds/gateway-inference-extension-v1alpha2pool.yaml >> config/llmisvc/gateway-inference-extension.yaml
	cat test/crds/gateway-inference-extension-v1alpha2pool.yaml >> test/crds/gateway-inference-extension.yaml

	# Move StorageContainer CRD to storagecontainer folder
	mv config/crd/full/serving.kserve.io_clusterstoragecontainers.yaml config/crd/full/clusterstoragecontainer/serving.kserve.io_clusterstoragecontainers.yaml
	
	# Move LLMISVC CRD to llmisvc folder	                   
	mv config/crd/full/serving.kserve.io_llminferenceservices.yaml config/crd/full/llmisvc/serving.kserve.io_llminferenceservices.yaml
	mv config/crd/full/serving.kserve.io_llminferenceserviceconfigs.yaml config/crd/full/llmisvc/serving.kserve.io_llminferenceserviceconfigs.yaml
	
	# Move LocalModel CRD to localmodel folder
	mv config/crd/full/serving.kserve.io_localmodelcaches.yaml config/crd/full/localmodel/serving.kserve.io_localmodelcaches.yaml
	mv config/crd/full/serving.kserve.io_localmodelnamespacecaches.yaml config/crd/full/localmodel/serving.kserve.io_localmodelnamespacecaches.yaml
	mv config/crd/full/serving.kserve.io_localmodelnodegroups.yaml config/crd/full/localmodel/serving.kserve.io_localmodelnodegroups.yaml
	mv config/crd/full/serving.kserve.io_localmodelnodes.yaml config/crd/full/localmodel/serving.kserve.io_localmodelnodes.yaml

	# Move KernelCache CRD to kernelcache folder
	mv config/crd/full/serving.kserve.io_kernelcaches.yaml config/crd/full/kernelcache/serving.kserve.io_kernelcaches.yaml
	mv config/crd/full/serving.kserve.io_kernelcachenodes.yaml config/crd/full/kernelcache/serving.kserve.io_kernelcachenodes.yaml
	mv config/crd/full/serving.kserve.io_kernelcachecaptures.yaml config/crd/full/kernelcache/serving.kserve.io_kernelcachecaptures.yaml
	mv config/crd/full/serving.kserve.io_kernelcachenodegroups.yaml config/crd/full/kernelcache/serving.kserve.io_kernelcachenodegroups.yaml
		
	@# An existing deepcopy file keeps its copyright year, a new one gets CURRENT_YEAR.
	@for api in v1alpha1 v1alpha2 v1beta1; do \
		dir=pkg/apis/serving/$$api; \
		year=$$(awk '/^Copyright [0-9]+ The KServe Authors/ { print $$2; exit }' "$$dir/zz_generated.deepcopy.go" 2>/dev/null); \
		$(CONTROLLER_GEN) object:headerFile=hack/boilerplate.go.txt,year="$${year:-$(CURRENT_YEAR)}" paths="./$$dir" || exit 1; \
	done

	# Strip schema validation from embedded external types in LLMInferenceServiceConfig.
	# These subtrees contain Go template expressions (e.g. {{ .GlobalConfig.ModelBasedRoutingHeaderName }})
	# that fail upstream schema validation at apply time. Recursive descent per subtree survives
	# schema restructuring across Gateway API / GIE / core API version bumps.
	# Then remove validation for the LLMInferenceServiceConfig and LLMInferenceService APIs so that
	# we can override only specific values (both versions).
	# All edits to a file run as one sequential yq pipeline: each yq invocation re-parses the
	# multi-megabyte CRD.
	@set -e; strip='.'; required='.'; \
	for ver in 0 1; do \
		base=".spec.versions[$$ver].schema.openAPIV3Schema.properties.spec.properties"; \
		for path in \
			"$$base.router.properties.route.properties.http.properties.spec" \
			"$$base.router.properties.scheduler.properties.pool.properties.spec" \
			"$$base.template" \
			"$$base.worker" \
			"$$base.prefill.properties.template" \
			"$$base.prefill.properties.worker" \
			"$$base.router.properties.scheduler.properties.template" \
			"$$base.router.properties.scheduler.properties.tokenizer.properties.template"; \
		do \
			for field in pattern x-kubernetes-validations minLength minItems minProperties; do \
				strip="$$strip | (($$path | .. | select(has(\"$$field\"))) |= del(.$$field))"; \
			done; \
		done; \
	done; \
	for ver in 0 1; do \
		base=".spec.versions[$$ver].schema.openAPIV3Schema.properties.spec.properties"; \
		for path in worker template prefill.properties.template prefill.properties.worker router.properties.scheduler.properties.template; do \
			required="$$required | del($$base.$$path.required)"; \
		done; \
	done; \
	$(YQ) "$$strip | $$required" -i config/crd/full/llmisvc/serving.kserve.io_llminferenceserviceconfigs.yaml; \
	$(YQ) "$$required" -i config/crd/full/llmisvc/serving.kserve.io_llminferenceservices.yaml

	# InferenceService CRD edits, as one yq pipeline:
	#remove the required property on framework as name field needs to be optional
	#remove ephemeralContainers properties for compress crd size https://github.com/kubeflow/kfserving/pull/1141#issuecomment-714170602
	#knative does not allow setting port on liveness or readiness probe
	#With v1 and newer kubernetes protocol requires default
	@spec='.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties'; \
	$(YQ) -i "del($$spec.*.properties.*.required) \
		| del($$spec.*.properties.ephemeralContainers) \
		| del($$spec.*.properties.*.properties.readinessProbe.properties.httpGet.required) \
		| del($$spec.*.properties.*.properties.livenessProbe.properties.httpGet.required) \
		| del($$spec.*.properties.*.properties.readinessProbe.properties.tcpSocket.required) \
		| del($$spec.*.properties.*.properties.livenessProbe.properties.tcpSocket.required) \
		| del($$spec.*.properties.containers.items.properties.livenessProbe.properties.httpGet.required) \
		| del($$spec.*.properties.containers.items.properties.readinessProbe.properties.httpGet.required) \
		| (($$spec | .. | select(has(\"protocol\")) | .protocol.default) = \"TCP\")" \
		config/crd/full/serving.kserve.io_inferenceservices.yaml
	@$(YQ) '(.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties | .. | select(has("protocol")) | .protocol.default) = "TCP"' -i config/crd/full/serving.kserve.io_clusterservingruntimes.yaml
	@$(YQ) '(.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties | .. | select(has("protocol")) | .protocol.default) = "TCP"' -i config/crd/full/serving.kserve.io_servingruntimes.yaml
	
	# Copy kserve and llmisvc crds (with conversion webhook patches applied via kustomize), and
	# concatenate the full crds into the test folder. Each kustomization is built once.
	@set -e; tmp=$$(mktemp -d); trap 'rm -rf "$$tmp"' EXIT; \
	$(KUSTOMIZE) build config/crd/full > "$$tmp/kserve.yaml"; \
	$(KUSTOMIZE) build config/crd/full/llmisvc > "$$tmp/llmisvc.yaml"; \
	for crd in inferenceservices trainedmodels clusterservingruntimes servingruntimes inferencegraphs; do \
		$(YQ) -e "select(.metadata.name == \"$$crd.serving.kserve.io\")" "$$tmp/kserve.yaml" > charts/kserve-crd/templates/serving.kserve.io_$$crd.yaml; \
	done; \
	for crd in llminferenceservices llminferenceserviceconfigs; do \
		$(YQ) -e "select(.metadata.name == \"$$crd.serving.kserve.io\")" "$$tmp/llmisvc.yaml" > charts/kserve-llmisvc-crd/templates/serving.kserve.io_$$crd.yaml; \
	done; \
	{ \
		cat "$$tmp/kserve.yaml"; echo "---"; \
		$(KUSTOMIZE) build config/crd/full/clusterstoragecontainer; echo "---"; \
		cat "$$tmp/llmisvc.yaml"; echo "---"; \
		$(KUSTOMIZE) build config/crd/full/localmodel; echo "---"; \
		$(KUSTOMIZE) build config/crd/full/kernelcache; \
	} > test/crds/serving.kserve.io_all_crds.yaml
	cp config/crd/full/clusterstoragecontainer/serving.kserve.io_clusterstoragecontainers.yaml charts/kserve-crd/files/
	cp config/crd/full/clusterstoragecontainer/serving.kserve.io_clusterstoragecontainers.yaml charts/kserve-llmisvc-crd/files/
	cp -f config/crd/full/localmodel/*.yaml charts/kserve-localmodel-crd/templates/
	cp -f config/crd/full/kernelcache/*.yaml charts/kserve-localmodel-crd/templates/
	rm charts/kserve-localmodel-crd/templates/kustomization.yaml
	
	# Generate minimal crd
	./hack/minimal-crdgen.sh
	
	# Copy kserve and llmisvc minimal crds (with conversion webhook patches applied via kustomize)
	@set -e; tmp=$$(mktemp -d); trap 'rm -rf "$$tmp"' EXIT; \
	$(KUSTOMIZE) build config/crd/minimal > "$$tmp/kserve.yaml"; \
	$(KUSTOMIZE) build config/crd/minimal/llmisvc > "$$tmp/llmisvc.yaml"; \
	for crd in inferenceservices trainedmodels clusterservingruntimes servingruntimes inferencegraphs; do \
		$(YQ) -e "select(.metadata.name == \"$$crd.serving.kserve.io\")" "$$tmp/kserve.yaml" > charts/kserve-crd-minimal/templates/serving.kserve.io_$$crd.yaml; \
	done; \
	for crd in llminferenceservices llminferenceserviceconfigs; do \
		$(YQ) -e "select(.metadata.name == \"$$crd.serving.kserve.io\")" "$$tmp/llmisvc.yaml" > charts/kserve-llmisvc-crd-minimal/templates/serving.kserve.io_$$crd.yaml; \
	done
	cp -f config/crd/minimal/localmodel/*.yaml charts/kserve-localmodel-crd-minimal/templates/
	cp -f config/crd/minimal/kernelcache/*.yaml charts/kserve-localmodel-crd-minimal/templates/
	cp -f config/crd/minimal/clusterstoragecontainer/serving.kserve.io_clusterstoragecontainers.yaml charts/kserve-crd-minimal/files/
	cp -f config/crd/minimal/clusterstoragecontainer/serving.kserve.io_clusterstoragecontainers.yaml charts/kserve-llmisvc-crd-minimal/files/
	rm charts/kserve-localmodel-crd-minimal/templates/kustomization.yaml
	
    # Copy Test inferenceconfig configmap to test overlay
	cp config/configmap/inferenceservice.yaml config/overlays/test/configmap/inferenceservice.yaml

generate: controller-gen helm-docs ## After API type changes: clientsets, OpenAPI, Python SDK, chart READMEs. Needs Java.
	@# Preserve existing copyright years across regeneration.
	@grep -rn 'Copyright [0-9]\{4\} The KServe Authors' --include='*.go' --include='*.py' \
		pkg/ cmd/ python/ 2>/dev/null | \
		sed -n 's/^\(.*\):[0-9]*:.*Copyright \([0-9]\{4\}\).*/\1\t\2/p' | \
		sort -u > /tmp/copyright_years_cache
	hack/update-codegen.sh
	hack/update-openapigen.sh
	COPYRIGHT_YEARS_CACHE=/tmp/copyright_years_cache hack/python-sdk/client-gen.sh
	@python3 hack/setup/preserve-copyright-years.py /tmp/copyright_years_cache
	@rm -f /tmp/copyright_years_cache
	$(HELM_DOCS) --chart-search-root=charts --output-file=README.md

uv-lock: $(UV) ## After Python dependency changes: re-lock the python/ uv.lock files. Needs network.
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

# Sync common resource helpers to charts that need them (must run before helm package)
sync-helm-common-resource-helpers:
	@echo "Syncing common resource helpers to charts..."
	@for chart in kserve-resources kserve-llmisvc-resources; do \
		cp charts/_common/_common.tpl charts/$$chart/templates/_common.tpl; \
		echo "  ✓ Copied to charts/$$chart/templates/_common.tpl"; \
	done

# Sync multi-resource helpers to charts that need them (must run before helm package)
sync-helm-multi-resource-helpers:
	@echo "Syncing multi-resource helpers to charts..."
	@for chart in kserve-resources kserve-llmisvc-resources kserve-localmodel-resources; do \
		cp charts/_common/_resources.tpl charts/$$chart/templates/_resources.tpl; \
		echo "  ✓ Copied to charts/$$chart/templates/_resources.tpl"; \
	done

boilerplate:
	hack/boilerplate.sh

apidocs:
	${ENGINE} buildx build ${ARCH} -f docs/apis/Dockerfile --rm -t apidocs-gen . && \
	${ENGINE} run -it --rm -v $(CURDIR)/pkg/apis:/go/src/github.com/kserve/kserve/pkg/apis -v ${PWD}/docs/apis:/go/gen-crd-api-reference-docs/apidocs apidocs-gen

# Extension point for distro-specific manifest generation.
.PHONY: manifests-distro
manifests-distro:
