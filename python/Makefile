PROVISIONER_IMG ?= kfserving-provisioner:latest

docker-build-provisioner:
	docker build . -t ${PROVISIONER_IMG} -f provisioner.Dockerfile

docker-push-provisioner:
	docker push ${PROVISIONER_IMG}
