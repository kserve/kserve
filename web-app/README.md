# Models web app

This web app is responsible for allowing the user to manipulate the Model Servers in their Kubeflow cluster. To achieve this it provides a user friendly way to handle the lifecycle of `InferenceService` CRs.

The web app currently works with `v1beta1` versions of `InferenceService` objects.

## Development

The frontend is build with [Angular](https://angular.io/) and the backend is written with the Python [Flask](https://flask.palletsprojects.com/en/1.1.x/) framework.

This web app is utilizing common code from the [kubeflow/kubeflow](https://github.com/kubeflow/kubeflow/tree/master/components/crud-web-apps/common) repository. We want to enforce the same UX across our different Kubeflow web apps and also keep them in the same development state. In order to achieve this the apps will be using this shared common code.

This will require us to fetch this common code when we want to either build the app locally or in a container image.

In order to run the app locally you will need to:
1. Build the frontend, in watch mode
2. Run the backend

The `npm run build:watch` command will build the frontend artifacts inside the backend's `static` folder for serving. So in order to see the app you'll need to start the backend and connect to `localhost:5000`.

Requirements:
* node 12.0.0
* python 3.7

### Frontend
```bash
# build the common library
cd $KUBEFLOW_REPO/components/crud-web-apps/common/frontend/kubeflow-common-lib
npm i
npm run build
cd dist/kubeflow
npm link

# run the app frontend
cd $KFSERVING_REPO/web-app/frontend
npm i
npm link kubeflow
npm run build:watch
```

### Backend

#### run it locally
```bash
# create a virtual env and install deps
# https://packaging.python.org/guides/installing-using-pip-and-virtual-environments/
cd $KFSERVING_REPO/web-app/backend
python3.7 -m pip install --user virtualenv
python3.7 -m venv web-apps-dev
source web-apps-dev/bin/activate

# install the deps on the activated virtual env
KUBEFLOW_REPO="/path/to/kubeflow/kubeflow" make -C backend install-deps

# run the backend
make -C backend run-dev
```

