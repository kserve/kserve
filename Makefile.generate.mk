# Code/manifest generation and artifact sync.

.PHONY: sync-deps
sync-deps:
	@@python3 hack/setup/scripts/generate-versions-from-gomod.py --no-cache

.PHONY: sync-img-env
sync-img-env:
	@python3 hack/setup/scripts/generate-images-sh.py

generate-quick-install-scripts: validate-infra-scripts $(PYTHON_VENV)
	@$(PYTHON_BIN)/pip install -q -r hack/setup/scripts/install-script-generator/requirements.txt
	@$(PYTHON_BIN)/python hack/setup/scripts/install-script-generator/generator.py

generate-chart-manifests:
	@bash hack/setup/scripts/generate_chart_manifests.sh
	make lint-helm-charts
	make verify-helm-helpers-consistency

# Generate manifests e.g. CRD, RBAC etc.
manifests: controller-gen kustomize yq
	@$(CONTROLLER_GEN) $(CRD_OPTIONS) paths=./pkg/apis/serving/... output:crd:dir=config/crd/full	
	@$(CONTROLLER_GEN) rbac:roleName=kserve-manager-role paths={./pkg/controller/v1alpha1/inferencegraph,./pkg/controller/v1alpha1/trainedmodel,./pkg/controller/v1beta1/inferenceservice} output:rbac:artifacts:config=config/rbac
	@$(CONTROLLER_GEN) rbac:roleName=kserve-llmisvc-manager-role paths=./pkg/controller/v1alpha2/llmisvc output:rbac:artifacts:config=config/rbac/llmisvc
	@$(CONTROLLER_GEN) rbac:roleName=kserve-localmodel-manager-role paths=./pkg/controller/v1alpha1/localmodel output:rbac:artifacts:config=config/rbac/localmodel
	@$(CONTROLLER_GEN) rbac:roleName=kserve-localmodelnode-agent-role paths=./pkg/controller/v1alpha1/localmodelnode output:rbac:artifacts:config=config/rbac/localmodelnode
	# Hook for distro-specific manifest generation (override via Makefile.overrides.mk).
	@$(MAKE) manifests-distro

	# DO NOT COPY to helm chart. It needs to be created before the Envoy Gateway or you will need to restart the Envoy Gateway controller.
	# The llmisvc helm chart needs to be installed after the Envoy Gateway as well, so it needs to be created before the llmisvc helm chart.
	# Pull upstream GIE v1 CRDs (InferencePool, etc.) from release artifact
	curl -sL https://github.com/kubernetes-sigs/gateway-api-inference-extension/releases/download/$(GIE_VERSION)/v1-manifests.yaml > config/llmisvc/gateway-inference-extension.yaml
	# Append llm-d.ai CRDs (InferenceObjective, InferenceModelRewrite) from llm-d-router release
	@echo "---" >> config/llmisvc/gateway-inference-extension.yaml
	curl -sL https://github.com/llm-d/llm-d-router/releases/download/$(LLMD_ROUTER_VERSION)/manifests.yaml >> config/llmisvc/gateway-inference-extension.yaml
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
		
	@$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt",year=$(CURRENT_YEAR) paths=./pkg/apis/serving/v1alpha1
	@$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt",year=$(CURRENT_YEAR) paths=./pkg/apis/serving/v1alpha2
	@$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt",year=$(CURRENT_YEAR) paths=./pkg/apis/serving/v1beta1

	# Strip schema validation from embedded external types in LLMInferenceServiceConfig.
	# These subtrees contain Go template expressions (e.g. {{ .GlobalConfig.ModelBasedRoutingHeaderName }})
	# that fail upstream schema validation at apply time. Recursive descent per subtree survives
	# schema restructuring across Gateway API / GIE / core API version bumps.
	@for ver in 0 1; do \
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
				$(YQ) "($$path | .. | select(has(\"$$field\"))) |= del(.$$field)" -i config/crd/full/llmisvc/serving.kserve.io_llminferenceserviceconfigs.yaml; \
			done; \
		done; \
	done
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
	
	# Copy kserve crd (with conversion webhook patches applied via kustomize)
	$(KUSTOMIZE) build config/crd/full | $(YQ) 'select(.metadata.name == "inferenceservices.serving.kserve.io")' > charts/kserve-crd/templates/serving.kserve.io_inferenceservices.yaml
	$(KUSTOMIZE) build config/crd/full | $(YQ) 'select(.metadata.name == "trainedmodels.serving.kserve.io")' > charts/kserve-crd/templates/serving.kserve.io_trainedmodels.yaml
	$(KUSTOMIZE) build config/crd/full | $(YQ) 'select(.metadata.name == "clusterservingruntimes.serving.kserve.io")' > charts/kserve-crd/templates/serving.kserve.io_clusterservingruntimes.yaml
	$(KUSTOMIZE) build config/crd/full | $(YQ) 'select(.metadata.name == "servingruntimes.serving.kserve.io")' > charts/kserve-crd/templates/serving.kserve.io_servingruntimes.yaml
	$(KUSTOMIZE) build config/crd/full | $(YQ) 'select(.metadata.name == "inferencegraphs.serving.kserve.io")' > charts/kserve-crd/templates/serving.kserve.io_inferencegraphs.yaml
	cp config/crd/full/clusterstoragecontainer/serving.kserve.io_clusterstoragecontainers.yaml charts/kserve-crd/files/
	cp config/crd/full/clusterstoragecontainer/serving.kserve.io_clusterstoragecontainers.yaml charts/kserve-llmisvc-crd/files/
	cp -f config/crd/full/localmodel/*.yaml charts/kserve-localmodel-crd/templates/
	rm charts/kserve-localmodel-crd/templates/kustomization.yaml
	
	# Copy llmisvc crd (with conversion webhook patches applied via kustomize).
	# Written to files/ (not templates/) so the chart's render-time transforms
	# in templates/serving.kserve.io_*.yaml survive regeneration.
	$(KUSTOMIZE) build config/crd/full/llmisvc | $(YQ) 'select(.metadata.name == "llminferenceservices.serving.kserve.io")' > charts/kserve-llmisvc-crd/files/serving.kserve.io_llminferenceservices.yaml
	$(KUSTOMIZE) build config/crd/full/llmisvc | $(YQ) 'select(.metadata.name == "llminferenceserviceconfigs.serving.kserve.io")' > charts/kserve-llmisvc-crd/files/serving.kserve.io_llminferenceserviceconfigs.yaml
	
	# Copy the full crd to the test folder
	$(KUSTOMIZE) build config/crd/full > test/crds/serving.kserve.io_all_crds.yaml
	echo "---" >> test/crds/serving.kserve.io_all_crds.yaml
	$(KUSTOMIZE) build config/crd/full/clusterstoragecontainer >> test/crds/serving.kserve.io_all_crds.yaml
	echo "---" >> test/crds/serving.kserve.io_all_crds.yaml
	$(KUSTOMIZE) build config/crd/full/llmisvc >> test/crds/serving.kserve.io_all_crds.yaml
	echo "---" >> test/crds/serving.kserve.io_all_crds.yaml
	$(KUSTOMIZE) build config/crd/full/localmodel >> test/crds/serving.kserve.io_all_crds.yaml
	
	# Generate minimal crd
	./hack/minimal-crdgen.sh
	
	# Copy kserve minimal crd (with conversion webhook patches applied via kustomize)
	$(KUSTOMIZE) build config/crd/minimal | $(YQ) 'select(.metadata.name == "inferenceservices.serving.kserve.io")' > charts/kserve-crd-minimal/templates/serving.kserve.io_inferenceservices.yaml
	$(KUSTOMIZE) build config/crd/minimal | $(YQ) 'select(.metadata.name == "trainedmodels.serving.kserve.io")' > charts/kserve-crd-minimal/templates/serving.kserve.io_trainedmodels.yaml
	$(KUSTOMIZE) build config/crd/minimal | $(YQ) 'select(.metadata.name == "clusterservingruntimes.serving.kserve.io")' > charts/kserve-crd-minimal/templates/serving.kserve.io_clusterservingruntimes.yaml
	$(KUSTOMIZE) build config/crd/minimal | $(YQ) 'select(.metadata.name == "servingruntimes.serving.kserve.io")' > charts/kserve-crd-minimal/templates/serving.kserve.io_servingruntimes.yaml
	$(KUSTOMIZE) build config/crd/minimal | $(YQ) 'select(.metadata.name == "inferencegraphs.serving.kserve.io")' > charts/kserve-crd-minimal/templates/serving.kserve.io_inferencegraphs.yaml
	cp -f config/crd/minimal/localmodel/*.yaml charts/kserve-localmodel-crd-minimal/templates/
	cp -f config/crd/minimal/clusterstoragecontainer/serving.kserve.io_clusterstoragecontainers.yaml charts/kserve-crd-minimal/files/
	cp -f config/crd/minimal/clusterstoragecontainer/serving.kserve.io_clusterstoragecontainers.yaml charts/kserve-llmisvc-crd-minimal/files/
	rm charts/kserve-localmodel-crd-minimal/templates/kustomization.yaml

	# Copy minimal llmisvc crd (with conversion webhook patches applied via kustomize)
	$(KUSTOMIZE) build config/crd/minimal/llmisvc | $(YQ) 'select(.metadata.name == "llminferenceservices.serving.kserve.io")' > charts/kserve-llmisvc-crd-minimal/templates/serving.kserve.io_llminferenceservices.yaml
	$(KUSTOMIZE) build config/crd/minimal/llmisvc | $(YQ) 'select(.metadata.name == "llminferenceserviceconfigs.serving.kserve.io")' > charts/kserve-llmisvc-crd-minimal/templates/serving.kserve.io_llminferenceserviceconfigs.yaml
	
    # Copy Test inferenceconfig configmap to test overlay
	cp config/configmap/inferenceservice.yaml config/overlays/test/configmap/inferenceservice.yaml

# Generate code
generate: controller-gen helm-docs
	@# Preserve existing copyright years across regeneration.
	@grep -rn 'Copyright [0-9]\{4\} The KServe Authors' --include='*.go' --include='*.py' \
		pkg/ cmd/ python/ 2>/dev/null | \
		sed -n 's/^\(.*\):[0-9]*:.*Copyright \([0-9]\{4\}\).*/\1\t\2/p' | \
		sort -u > /tmp/copyright_years_cache
	hack/update-codegen.sh
	hack/update-openapigen.sh
	hack/python-sdk/client-gen.sh
	@while read -r line; do \
		f=$$(echo "$$line" | cut -f1); year=$$(echo "$$line" | cut -f2); \
		if [ -f "$$f" ]; then sed -i "s/Copyright [0-9]\{4\} The KServe Authors/Copyright $$year The KServe Authors/" "$$f"; fi; \
	done < /tmp/copyright_years_cache
	@rm -f /tmp/copyright_years_cache
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
