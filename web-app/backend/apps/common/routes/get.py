from flask import request

from kubeflow.kubeflow.crud_backend import api, logging

from .. import utils, versions
from . import bp

log = logging.getLogger(__name__)


@bp.route("/api/namespaces/<namespace>/inferenceservices")
def get_inference_services(namespace):
    gvk = versions.inference_service_gvk()
    inference_services = api.list_custom_rsrc(**gvk, namespace=namespace)

    return api.success_response("inferenceServices",
                                inference_services["items"])


@bp.route("/api/namespaces/<namespace>/inferenceservices/<name>")
def get_inference_service(namespace, name):
    inference_service = api.get_custom_rsrc(**versions.inference_service_gvk(),
                                            namespace=namespace, name=name)
    if request.args.get("logs", "false") == "true":
        # find the logs
        return api.success_response(
            "serviceLogs", get_inference_service_logs(inference_service)
        )

    return api.success_response("inferenceService", inference_service)


def get_inference_service_logs(svc):
    namespace = svc["metadata"]["namespace"]
    components = request.args.getlist("component")

    log.info(components)

    # dictionary{component: [pod-names]}
    component_pods_dict = utils.get_inference_service_pods(svc, components)

    if len(component_pods_dict.keys()) == 0:
        return {}

    resp = {}
    logging.info("Component pods: %s", component_pods_dict)
    for component, pods in component_pods_dict.items():
        if component not in resp:
            resp[component] = []

        for pod in pods:
            logs = api.get_pod_logs(namespace, pod, "kserve-container",
                                    auth=False)
            resp[component].append({"podName": pod,
                                    "logs": logs.split("\n")})
    return resp


@bp.route("/api/namespaces/<namespace>/knativeServices/<name>")
def get_knative_service(namespace, name):
    svc = api.get_custom_rsrc(**versions.KNATIVE_SERVICE, namespace=namespace,
                              name=name)

    return api.success_response("knativeService", svc)


@bp.route("/api/namespaces/<namespace>/configurations/<name>")
def get_knative_configuration(namespace, name):
    svc = api.get_custom_rsrc(**versions.KNATIVE_CONF, namespace=namespace,
                              name=name)

    return api.success_response("knativeConfiguration", svc)


@bp.route("/api/namespaces/<namespace>/revisions/<name>")
def get_knative_revision(namespace, name):
    svc = api.get_custom_rsrc(**versions.KNATIVE_REVISION, namespace=namespace,
                              name=name)

    return api.success_response("knativeRevision", svc)


@bp.route("/api/namespaces/<namespace>/routes/<name>")
def get_knative_route(namespace, name):
    svc = api.get_custom_rsrc(**versions.KNATIVE_ROUTE, namespace=namespace,
                              name=name)

    return api.success_response("knativeRoute", svc)
