from kubeflow.kubeflow.crud_backend import api, logging

from .. import versions
from . import bp

log = logging.getLogger(__name__)


@bp.route(
    "/api/namespaces/<namespace>/inferenceservices/<inference_service>",
    methods=["DELETE"],
)
def delete_inference_service(inference_service, namespace):
    log.info("Deleting InferenceService %s/%s'", namespace, inference_service)
    gvk = versions.inference_service_gvk()
    api.delete_custom_rsrc(**gvk,
                           name=inference_service, namespace=namespace)
    return api.success_response("message",
                                "InferenceService %s/%s successfully deleted."
                                % (namespace, inference_service))
