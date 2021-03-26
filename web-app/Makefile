IMG ?= gcr.io/kfserving/models-web-app

# We want the git tag to be the last commit to this directory so we don't
# bump the image on unrelated changes.
GIT_TAG ?= $(shell git log -n 1 --pretty=format:"%h" ./)

docker-build:
	docker build -t $(IMG):$(GIT_TAG) .

docker-push:
	docker push $(IMG):$(GIT_TAG)

image: docker-build docker-push
