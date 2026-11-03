# Local development: build binaries, run, deploy/undeploy.

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
	kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/${GATEWAY_API_VERSION}/standard-install.yaml
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
	hack/setup/infra/manage.lws-operator.sh
	hack/setup/infra/gateway-api/manage.gateway-api-extension-crd.sh
	hack/setup/infra/manage.envoy-gateway-helm.sh
	hack/setup/infra/manage.envoy-ai-gateway-helm.sh
	hack/setup/infra/gateway-api/manage.gateway-api-gwclass.sh
	hack/setup/infra/gateway-api/manage.gateway-api-gw.sh
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

deploy-dev-qpext: docker-build-push-qpext
	kubectl patch cm config-deployment -n knative-serving --type merge --patch '{"data": {"queue-sidecar-image": "${KO_DOCKER_REPO}/${QPEXT_IMG}"}}'

# Build and push controller/localmodel images, then install KServe + LocalModel on kind via kustomize.
# Uses KO_DOCKER_REPO/TAG for image overrides; skips configmap image rewrites (UPDATE_CONFIGMAP_IMAGES=false).
.PHONY: deploy-dev-kind-localmodel
deploy-dev-kind-localmodel: docker-build docker-push docker-build-localmodel docker-push-localmodel 
	SET_KSERVE_REGISTRY=$$KO_DOCKER_REPO SET_KSERVE_VERSION=$$TAG \
	ENABLE_KSERVE=true ENABLE_LOCALMODEL=true UPDATE_CONFIGMAP_IMAGES=false \
	./hack/setup/infra/manage.kserve-kustomize.sh
