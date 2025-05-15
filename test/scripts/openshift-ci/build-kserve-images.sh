#!/usr/bin/env bash

BUILDER=${BUILDER:-"docker"}
GITHUB_SHA=${GITHUB_SHA:-"master"}

if [ -z "${QUAY_REPO}" ]; then
    echo "Error: QUAY_REPO environment variable is not set"
    exit 1
fi
export KO_DOCKER_REPO=$QUAY_REPO/kserve

# Function to check command status
check_status() {
    if [ $? -ne 0 ]; then
        echo "Error: $1 failed"
        exit 1
    fi
    echo "$1 completed successfully"
}

# Build and push Sklearn image
echo "Building Sklearn image..."
make docker-build-sklearn #git@github.com:opendatahub-io/kserve.git
check_status "Sklearn image build"

SKLEARN_DOCKER_IMAGE=$KO_DOCKER_REPO/sklearnserver:$GITHUB_SHA
$BUILDER tag $KO_DOCKER_REPO/sklearnserver $SKLEARN_DOCKER_IMAGE
check_status "Sklearn image tag"

$BUILDER push $SKLEARN_DOCKER_IMAGE
check_status "Sklearn image push"
export SKLEARN_IMAGE=$SKLEARN_DOCKER_IMAGE

# Build and push Storage Initializer image
echo "Building Storage Initializer image..."
make docker-build-storageInitializer
check_status "Storage Initializer image build"

STORAGE_INITIALIZER_DOCKER_IMAGE=$KO_DOCKER_REPO/kserve-storage-initializer:$GITHUB_SHA
$BUILDER tag $KO_DOCKER_REPO/storage-initializer $STORAGE_INITIALIZER_DOCKER_IMAGE
check_status "Storage Initializer image tag"

$BUILDER push $STORAGE_INITIALIZER_DOCKER_IMAGE
check_status "Storage Initializer image push"
export STORAGE_INITIALIZER_IMAGE=$STORAGE_INITIALIZER_DOCKER_IMAGE

# Build and push KServe Agent image
echo "Building KServe Agent image..."
make docker-build-agent
check_status "KServe Agent image build"

KSERVE_AGENT_DOCKER_IMAGE=$KO_DOCKER_REPO/kserve-agent:$GITHUB_SHA
$BUILDER tag $KO_DOCKER_REPO/agent $KSERVE_AGENT_DOCKER_IMAGE
check_status "KServe Agent image tag"

$BUILDER push $KSERVE_AGENT_DOCKER_IMAGE
check_status "KServe Agent image push"
export KSERVE_AGENT_IMAGE=$KSERVE_AGENT_DOCKER_IMAGE

# Build and push KServe Router image
echo "Building KServe Router image..."
make docker-build-router
check_status "KServe Router image build"

KSERVE_ROUTER_DOCKER_IMAGE=$KO_DOCKER_REPO/kserve-router:$GITHUB_SHA
$BUILDER tag $KO_DOCKER_REPO/router $KSERVE_ROUTER_DOCKER_IMAGE
check_status "KServe Router image tag"

$BUILDER push $KSERVE_ROUTER_DOCKER_IMAGE
check_status "KServe Router image push"
export KSERVE_ROUTER_IMAGE=$KSERVE_ROUTER_DOCKER_IMAGE

# Build and push KServe Controller image
echo "Building KServe Controller image..."
make docker-build
check_status "KServe Controller image build"

KSERVE_CONTROLLER_DOCKER_IMAGE=$QUAY_REPO/kserve-controller:$GITHUB_SHA
$BUILDER tag localhost/kserve-controller:latest $KSERVE_CONTROLLER_DOCKER_IMAGE
check_status "KServe Controller image tag"

$BUILDER push $KSERVE_CONTROLLER_DOCKER_IMAGE
check_status "KServe Controller image push"
export KSERVE_CONTROLLER_IMAGE=$KSERVE_CONTROLLER_DOCKER_IMAGE

echo "All images built and pushed successfully!"

