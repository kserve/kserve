from flask import current_app

KNATIVE_ROUTE = {"group": "serving.knative.dev",
                 "version": "v1",
                 "kind": "routes"}
KNATIVE_REVISION = {"group": "serving.knative.dev",
                    "version": "v1",
                    "kind": "revisions"}
KNATIVE_CONF = {"group": "serving.knative.dev",
                "version": "v1",
                "kind": "configurations"}
KNATIVE_SERVICE = {"group": "serving.knative.dev",
                   "version": "v1",
                   "kind": "services"}


def inference_service_gvk():
    try:
        version = current_app.config["APP_VERSION"]
        if version not in ['v1alpha2', 'v1beta1']:
            version = 'v1beta1'
    except RuntimeError:
        version = "v1alpha2"

    return {"group": "serving.kubeflow.org",
            "version": version,
            "kind": "inferenceservices"}
