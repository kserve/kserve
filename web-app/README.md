# Models web app

This web app is responsible for allowing the user to manipulate the Model Servers in their Kubeflow cluster. To achieve this it provides a user friendly way to handle the lifecycle of `InferenceService` CRs.

The web app currently works with `v1beta1` versions of `InferenceService` objects.

## Connect to the app

The web app is installed alongside the other KFServing components, either in the `kfserving-system` or in the `kubeflow` namespace. There is a `VirtualService` that exposes the app via an Istio Ingress Gateway. Depending on the installation environment the following Ingress Gateway will be used.

| Installation mode | IngressGateway |
| - | - |
| Standalone KFServing | knative-ingress-gateway.knative-serving |
| Kubeflow | kubeflow-gateway.kubeflow |

To access the app you will need to navigate with your browser to
```sh
${INGRESS_IP}/models/
```

Alternatively you can access the app via `kubectl port-forward`. In that case you will need to configure the app to:
1. Not perform any authorization checks, since there is no logged in user
2. Work under the `/` prefix
3. Disable Secure cookies, since the app will be exposed under plain http

You can apply the mentioned configurations by doing the following commands:
```bash
# edit the configmap
# CONFIG=config/overlays/kubeflow/kustomization.yaml
CONFIG=config/web-app/kustomization.yaml
vim ${CONFIG}

# Add the following env vars to the configMapGenerator's literals
# for kfserving-models-web-app-config
- APP_PREFIX=/
- APP_DISABLE_AUTH="True"
- APP_SECURE_COOKIES="False"

# reapply the kustomization
# kustomize build config/overlays/kubeflow | kubectl apply -f -
kustomize build config/default | kubectl apply -f -
```

## Configuration

The following is a list of ENV var that can be set for any web app that is using this base app.
| ENV Var | Default value | Description |
| - | - | - |
| APP_PREFIX | /models | Controls the app's prefix, by setting the [base-url](https://developer.mozilla.org/en-US/docs/Web/HTML/Element/base) element |
| APP_DISABLE_AUTH | False | Controls whether the app should use SubjectAccessReviews to ensure the user is authorized to perform an action |
| APP_SECURE_COOKIES | True | Controls whether the app should use [Secure](https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Set-Cookie#Secure) CSRF cookies. By default the app expects to be exposed with https |
| CSRF_SAMESITE | Strict| Controls the [SameSite value](https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Set-Cookie#SameSite) of the CSRF cookie |
| USERID_HEADER | kubeflow-userid | Header in each request that will contain the username of the logged in user |
| USERID_PREFIX | "" | Prefix to remove from the `USERID_HEADER` value to extract the logged in user name |

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
git checkout e6fdf51

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

