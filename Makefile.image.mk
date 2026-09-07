# Container image build and push (engine set by ENGINE, default docker).

# Build the docker image
docker-build:
	${ENGINE} buildx build ${ARCH} --load --build-arg GOTAGS=${GOTAGS} . -t ${KO_DOCKER_REPO}/${CONTROLLER_IMG}:${TAG}
	@echo "updating kustomize image patch file for manager resource"

# Push the docker image
docker-push:
	docker push ${KO_DOCKER_REPO}/${CONTROLLER_IMG}:${TAG}

docker-build-llmisvc:
	${ENGINE} buildx build ${ARCH} --load --build-arg GOTAGS=${GOTAGS} -t ${KO_DOCKER_REPO}/${LLMISVC_CONTROLLER_IMG}:${TAG} -f llmisvc-controller.Dockerfile .

docker-push-llmisvc: docker-build-llmisvc
	${ENGINE} push ${KO_DOCKER_REPO}/${LLMISVC_CONTROLLER_IMG}:${TAG}

docker-build-localmodel:
	${ENGINE} buildx build ${ARCH} --load --build-arg GOTAGS=${GOTAGS} -t ${KO_DOCKER_REPO}/${LOCALMODEL_CONTROLLER_IMG}:${TAG} -f localmodel.Dockerfile .

docker-push-localmodel: docker-build-localmodel
	${ENGINE} buildx build ${ARCH} --push --build-arg GOTAGS=${GOTAGS} -t ${KO_DOCKER_REPO}/${LOCALMODEL_CONTROLLER_IMG}:${TAG} -f localmodel.Dockerfile .

docker-build-localmodelnode-agent:
	${ENGINE} buildx build ${ARCH} --load --build-arg GOTAGS=${GOTAGS} -t ${KO_DOCKER_REPO}/${LOCALMODEL_AGENT_IMG}:${TAG} -f localmodel-agent.Dockerfile .

docker-push-localmodelnode-agent: docker-build-localmodelnode-agent
	${ENGINE} buildx build ${ARCH} --push --build-arg GOTAGS=${GOTAGS} -t ${KO_DOCKER_REPO}/${LOCALMODEL_AGENT_IMG}:${TAG} -f localmodel-agent.Dockerfile .

docker-build-agent:
	${ENGINE} buildx build ${ARCH} --build-arg GOTAGS=${GOTAGS} -f agent.Dockerfile . -t ${KO_DOCKER_REPO}/${AGENT_IMG}:${TAG}

docker-build-router:
	${ENGINE} buildx build ${ARCH} --build-arg GOTAGS=${GOTAGS} -f router.Dockerfile . -t ${KO_DOCKER_REPO}/${ROUTER_IMG}:${TAG}

docker-push-agent:
	${ENGINE} push ${KO_DOCKER_REPO}/${AGENT_IMG}:${TAG}

docker-push-router:
	${ENGINE} push ${KO_DOCKER_REPO}/${ROUTER_IMG}:${TAG}

docker-build-sklearn:
	cd python && ${ENGINE} buildx build ${ARCH} --build-arg BASE_IMAGE=${BASE_IMG} -t ${KO_DOCKER_REPO}/${SKLEARN_IMG}:${TAG} -f sklearn.Dockerfile .

docker-push-sklearn: docker-build-sklearn
	${ENGINE} push ${KO_DOCKER_REPO}/${SKLEARN_IMG}:${TAG}

docker-build-xgb:
	cd python && ${ENGINE} buildx build ${ARCH} --build-arg BASE_IMAGE=${BASE_IMG} -t ${KO_DOCKER_REPO}/${XGB_IMG}:${TAG} -f xgb.Dockerfile .

docker-push-xgb: docker-build-xgb
	${ENGINE} push ${KO_DOCKER_REPO}/${XGB_IMG}:${TAG}

docker-build-lgb:
	cd python && ${ENGINE} buildx build ${ARCH} --build-arg BASE_IMAGE=${BASE_IMG} -t ${KO_DOCKER_REPO}/${LGB_IMG}:${TAG} -f lgb.Dockerfile .

docker-push-lgb: docker-build-lgb
	${ENGINE} push ${KO_DOCKER_REPO}/${LGB_IMG}:${TAG}

docker-build-predictive:
	cd python && ${ENGINE} buildx build ${ARCH} --build-arg BASE_IMAGE=${BASE_IMG} -t ${KO_DOCKER_REPO}/${PREDICTIVE_IMG}:${TAG} -f predictiveserver.Dockerfile .

docker-push-predictive: docker-build-predictive
	cd python && ${ENGINE} buildx build ${ARCH} --push --build-arg BASE_IMAGE=${BASE_IMG} -t ${KO_DOCKER_REPO}/${PREDICTIVE_IMG}:${TAG} -f predictiveserver.Dockerfile .

docker-build-pmml:
	cd python && ${ENGINE} buildx build ${ARCH} --build-arg BASE_IMAGE=${PMML_BASE_IMG} -t ${KO_DOCKER_REPO}/${PMML_IMG}:${TAG} -f pmml.Dockerfile .

docker-push-pmml: docker-build-pmml
	${ENGINE} push ${KO_DOCKER_REPO}/${PMML_IMG}:${TAG}

docker-build-paddle:
	cd python && ${ENGINE} buildx build ${ARCH} --build-arg BASE_IMAGE=${BASE_IMG} -t ${KO_DOCKER_REPO}/${PADDLE_IMG}:${TAG} -f paddle.Dockerfile .

docker-push-paddle: docker-build-paddle
	${ENGINE} push ${KO_DOCKER_REPO}/${PADDLE_IMG}:${TAG}

docker-build-autogluon:
	cd python && ${ENGINE} buildx build ${ARCH} --build-arg BASE_IMAGE=${BASE_IMG} -t ${KO_DOCKER_REPO}/${AUTOGLUON_IMG}:${TAG} -f autogluon.Dockerfile .

docker-push-autogluon: docker-build-autogluon
	${ENGINE} push ${KO_DOCKER_REPO}/${AUTOGLUON_IMG}:${TAG}

docker-build-custom-model:
	cd python && ${ENGINE} buildx build ${ARCH} -t ${KO_DOCKER_REPO}/${CUSTOM_MODEL_IMG}:${TAG} -f custom_model.Dockerfile .

docker-push-custom-model: docker-build-custom-model
	docker push ${KO_DOCKER_REPO}/${CUSTOM_MODEL_IMG}:${TAG}

docker-build-custom-model-grpc:
	cd python && ${ENGINE} buildx build ${ARCH} -t ${KO_DOCKER_REPO}/${CUSTOM_MODEL_GRPC_IMG}:${TAG} -f custom_model_grpc.Dockerfile .

docker-push-custom-model-grpc: docker-build-custom-model-grpc
	${ENGINE} push ${KO_DOCKER_REPO}/${CUSTOM_MODEL_GRPC_IMG}:${TAG}

docker-build-custom-transformer:
	cd python && ${ENGINE} buildx build ${ARCH} -t ${KO_DOCKER_REPO}/${IMAGE_TRANSFORMER_IMG}:${TAG} -f custom_transformer.Dockerfile .

docker-push-custom-transformer: docker-build-custom-transformer
	${ENGINE} push ${KO_DOCKER_REPO}/${IMAGE_TRANSFORMER_IMG}:${TAG}

docker-build-custom-transformer-grpc:
	cd python && ${ENGINE} buildx build ${ARCH} -t ${KO_DOCKER_REPO}/${CUSTOM_TRANSFORMER_GRPC_IMG}:${TAG} -f custom_transformer_grpc.Dockerfile .

docker-push-custom-transformer-grpc: docker-build-custom-transformer-grpc
	${ENGINE} push ${KO_DOCKER_REPO}/${CUSTOM_TRANSFORMER_GRPC_IMG}:${TAG}

docker-build-aif:
	cd python && ${ENGINE} buildx build ${ARCH} -t ${KO_DOCKER_REPO}/${AIF_IMG}:${TAG} -f aiffairness.Dockerfile .

docker-push-aif: docker-build-aif
	${ENGINE} push ${KO_DOCKER_REPO}/${AIF_IMG}:${TAG}

docker-build-art:
	cd python && ${ENGINE} buildx build ${ARCH} -t ${KO_DOCKER_REPO}/${ART_IMG}:${TAG} -f artexplainer.Dockerfile .

docker-push-art: docker-build-art
	${ENGINE} push ${KO_DOCKER_REPO}/${ART_IMG}:${TAG}

docker-build-storageInitializer:
	cd python && ${ENGINE} buildx build ${ARCH} --load --build-arg BASE_IMAGE=${BASE_IMG} -t ${KO_DOCKER_REPO}/${STORAGE_INIT_IMG}:${TAG} -f storage-initializer.Dockerfile .

docker-push-storageInitializer: docker-build-storageInitializer
	${ENGINE} push ${KO_DOCKER_REPO}/${STORAGE_INIT_IMG}:${TAG}

docker-build-qpext:
	${ENGINE} buildx build ${ARCH} -t ${KO_DOCKER_REPO}/${QPEXT_IMG}:${TAG} -f qpext/qpext.Dockerfile .

docker-build-push-qpext: docker-build-qpext
	${ENGINE} push ${KO_DOCKER_REPO}/${QPEXT_IMG}:${TAG}

docker-build-mcv:
	${ENGINE} buildx build ${ARCH} -t ${KO_DOCKER_REPO}/${MCV_IMG} -f kernelcache/mcv/mcv.Dockerfile kernelcache/mcv

docker-push-mcv: docker-build-mcv
	${ENGINE} push ${KO_DOCKER_REPO}/${MCV_IMG}

docker-build-success-200-isvc:
	cd python && ${ENGINE} buildx build ${ARCH} -t ${KO_DOCKER_REPO}/${SUCCESS_200_ISVC_IMG}:${TAG} -f success_200_isvc.Dockerfile .

docker-push-success-200-isvc: docker-build-success-200-isvc
	${ENGINE} push ${KO_DOCKER_REPO}/${SUCCESS_200_ISVC_IMG}:${TAG}

docker-build-error-node-404:
	cd python && ${ENGINE} buildx build ${ARCH} -t ${KO_DOCKER_REPO}/${ERROR_404_ISVC_IMG}:${TAG} -f error_404_isvc.Dockerfile .

docker-push-error-node-404: docker-build-error-node-404
	${ENGINE} push ${KO_DOCKER_REPO}/${ERROR_404_ISVC_IMG}:${TAG}

docker-build-huggingface:
	cd python && ${ENGINE} buildx build ${ARCH} -t ${KO_DOCKER_REPO}/${HUGGINGFACE_IMG}:${TAG} -f huggingface_server.Dockerfile .

docker-push-huggingface: docker-build-huggingface
	${ENGINE} push ${KO_DOCKER_REPO}/${HUGGINGFACE_IMG}:${TAG}

docker-build-huggingface-cpu:
	cd python && ${ENGINE} buildx build ${ARCH} -t ${KO_DOCKER_REPO}/${HUGGINGFACE_SERVER_CPU_IMG}:${TAG} -f huggingface_server_cpu.Dockerfile .

docker-push-huggingface-cpu: docker-build-huggingface-cpu
	${ENGINE} push ${KO_DOCKER_REPO}/${HUGGINGFACE_SERVER_CPU_IMG}:${TAG}
