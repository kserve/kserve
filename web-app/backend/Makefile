SHELL=bash

# In order to develop locally you will need to use common python code
# from the kubeflow/kubeflow repo. You can clone the repo in a specific
# folder and use the following ENV Var to configure it.
KUBEFLOW_REPO ?= /tmp/kubeflow
COMMON_BACKEND_DIR ?= ${KUBEFLOW_REPO}/components/crud-web-apps/common/backend

install-deps:
	pushd ${COMMON_BACKEND_DIR} && \
	pip install -e . && \
	popd
	pip install -r requirements.txt

run:
	APP_PREFIX=/models \
	gunicorn -w 3 --bind 0.0.0.0:5000 --access-logfile - entrypoint:app

run-dev:
	UI_FLAVOR=default \
	BACKEND_MODE=dev \
	APP_PREFIX=/ \
	gunicorn -w 3 --bind 0.0.0.0:5000 --access-logfile - entrypoint:app
