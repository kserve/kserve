# OpenShift targets for kserve midstream (setup, teardown, and E2E testing).
# Included from Makefile.overrides.mk.
#
# WARNING: These targets are intended for DEVELOPMENT and CI use only.
# Do NOT run them against production or shared clusters.  The undeploy-ocp
# target removes operators, namespaces, and cluster-scoped resources
# non-surgically; it will destroy workloads it did not create.

E2E_MARKER ?= predictor
E2E_PARALLELISM ?= 6
E2E_PROFILE ?=
SETUP_E2E ?= false
QUAY_REPO ?=
GITHUB_SHA ?= master

# Operator install mode: odh, rhoai, or empty (manual kustomize deploy).
OPERATOR_TYPE ?=
# Pin to a specific operator version (e.g. 3.4.0). When empty and CATALOG_SOURCE
# is an FBC fragment image, the version is auto-detected from the image tag.
OPERATOR_VERSION ?=
# FBC fragment image or CatalogSource name. Empty = default catalog.
# Example: quay.io/rhoai/rhoai-fbc-fragment:rhoai-3.4
CATALOG_SOURCE ?=
# Controls whether local branch manifests are copied into the operator PVC so the
# operator uses them instead of its bundled manifests. Only relevant for operator
# installs (OPERATOR_TYPE=odh|rhoai); in manual mode the local branch overlays
# are always used (there is no bundled artifact to fall back to).
COPY_PR_MANIFESTS ?= true
# Set to true to build and push local KServe images before setup, and use them
# in the test run. Requires QUAY_REPO to be set.
# NOTE: RUNNING_LOCAL and COPY_PR_MANIFESTS must be consistent for operator installs:
#   RUNNING_LOCAL=true  + COPY_PR_MANIFESTS=true  -> build & inject local images (default for local dev)
#   RUNNING_LOCAL=false + COPY_PR_MANIFESTS=false -> vanilla bundled operator (no local images)
#   RUNNING_LOCAL=true  + COPY_PR_MANIFESTS=false -> images are built but NOT injected -- avoid this
RUNNING_LOCAL ?= true
# Set to false to skip the docker build/push when images are already pushed (e.g. re-running setup).
# Only has effect when RUNNING_LOCAL=true.
BUILD_KSERVE_IMAGES ?= true

# Namespace where KServe controller runs. Derived from OPERATOR_TYPE when not set.
# odh/opendatahub -> opendatahub, rhoai/rhods -> redhat-ods-applications, empty -> kserve
ifeq ($(strip $(OPERATOR_TYPE)),odh)
  KSERVE_NAMESPACE ?= opendatahub
else ifeq ($(strip $(OPERATOR_TYPE)),opendatahub)
  KSERVE_NAMESPACE ?= opendatahub
else ifeq ($(strip $(OPERATOR_TYPE)),rhoai)
  KSERVE_NAMESPACE ?= redhat-ods-applications
else ifeq ($(strip $(OPERATOR_TYPE)),rhods)
  KSERVE_NAMESPACE ?= redhat-ods-applications
else
  KSERVE_NAMESPACE ?= kserve
endif

# Extra pytest flags passed directly to the test runner.
# Example: PYTEST_ARGS="-k test_sklearn_kserve"  (single smoke test)
#          PYTEST_ARGS='-k "test_sklearn_kserve or test_sklearn_v2"'
PYTEST_ARGS ?=

build-images-ocp: ## Build and push KServe images for E2E testing. Requires QUAY_REPO.
	QUAY_REPO="$(QUAY_REPO)" GITHUB_SHA="$(GITHUB_SHA)" \
	./test/scripts/openshift-ci/build-kserve-images.sh

deploy-ocp: ## Install operator and deploy KServe on OpenShift (no E2E scaffolding). Use OPERATOR_TYPE=odh|rhoai, or leave empty for manual kustomize deploy.
	OPERATOR_TYPE="$(strip $(OPERATOR_TYPE))" \
	OPERATOR_VERSION="$(strip $(OPERATOR_VERSION))" \
	CATALOG_SOURCE="$(strip $(CATALOG_SOURCE))" \
	COPY_PR_MANIFESTS="$(strip $(COPY_PR_MANIFESTS))" \
	RUNNING_LOCAL="$(strip $(RUNNING_LOCAL))" \
	QUAY_REPO="$(QUAY_REPO)" \
	GITHUB_SHA="$(GITHUB_SHA)" \
	./test/scripts/openshift-ci/setup-kserve.sh

deploy-e2e-ocp: ## Set up E2E test environment on OpenShift. Use OPERATOR_TYPE=odh, or leave empty for manual kustomize deploy.
	OPERATOR_TYPE="$(strip $(OPERATOR_TYPE))" \
	OPERATOR_VERSION="$(strip $(OPERATOR_VERSION))" \
	CATALOG_SOURCE="$(strip $(CATALOG_SOURCE))" \
	COPY_PR_MANIFESTS="$(strip $(COPY_PR_MANIFESTS))" \
	RUNNING_LOCAL="$(strip $(RUNNING_LOCAL))" \
	BUILD_KSERVE_IMAGES="$(strip $(BUILD_KSERVE_IMAGES))" \
	QUAY_REPO="$(QUAY_REPO)" \
	GITHUB_SHA="$(GITHUB_SHA)" \
	./test/scripts/openshift-ci/setup-e2e-tests.sh "$(E2E_MARKER)"

run-e2e-ocp: ## Run E2E tests. Set SETUP_E2E=false to skip cluster setup (assumes deploy-e2e-ocp already ran).
	SETUP_E2E="$(SETUP_E2E)" \
	OPERATOR_TYPE="$(strip $(OPERATOR_TYPE))" \
	KSERVE_NAMESPACE="$(strip $(KSERVE_NAMESPACE))" \
	RUNNING_LOCAL="$(strip $(RUNNING_LOCAL))" \
	PYTEST_ARGS="$(PYTEST_ARGS)" \
	./test/scripts/openshift-ci/run-e2e-tests.sh "$(E2E_MARKER)" "$(E2E_PARALLELISM)" "$(E2E_PROFILE)"

e2e-graph-ocp: ## Run graph E2E suite.
	$(MAKE) run-e2e-ocp E2E_MARKER="graph" E2E_PARALLELISM="$(E2E_PARALLELISM)" E2E_PROFILE=raw SETUP_E2E=true

e2e-raw-ocp: ## Run raw E2E suite (includes rawcipn phase).
	$(MAKE) run-e2e-ocp E2E_MARKER="raw" E2E_PARALLELISM="$(E2E_PARALLELISM)" E2E_PROFILE=raw SETUP_E2E=true
	oc patch configmaps -n $(KSERVE_NAMESPACE) inferenceservice-config \
	  --patch '{"data":{"service":"{\"serviceClusterIPNone\": true}"}}'
	sleep 5
	$(MAKE) run-e2e-ocp E2E_MARKER="rawcipn" E2E_PARALLELISM=1 E2E_PROFILE=raw SETUP_E2E=false

e2e-predictor-ocp: ## Run predictor E2E suite.
	$(MAKE) run-e2e-ocp E2E_MARKER="predictor or kserve_on_openshift" E2E_PARALLELISM="$(E2E_PARALLELISM)" E2E_PROFILE=raw SETUP_E2E=true

e2e-llmisvc-ocp: ## Run LLMISvc E2E suite.
	$(MAKE) run-e2e-ocp E2E_MARKER="llmisvc_core and cluster_cpu and not pvc_storage" E2E_PARALLELISM=2 E2E_PROFILE=llm-d SETUP_E2E=true

reset-e2e-ocp: ## Reset the test namespace for a fresh E2E rerun.
	./test/scripts/openshift-ci/setup-ci-namespace.sh

redeploy-e2e-ocp: undeploy-ocp deploy-e2e-ocp ## Tear down then re-deploy the full E2E environment (safe for operator switches).

undeploy-ocp: ## Tear down the entire OCP environment (operator, DSC, namespaces). See WARNING at top of file.
	./test/scripts/openshift-ci/teardown-e2e-setup.sh
