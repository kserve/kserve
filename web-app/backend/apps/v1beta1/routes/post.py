from flask import request

from kubeflow.kubeflow.crud_backend import api, decorators, logging

from ...common import versions
from . import bp

log = logging.getLogger(__name__)


@bp.route("/api/namespaces/<namespace>/inferenceservices", methods=["POST"])
@decorators.request_is_json_type
@decorators.required_body_params("apiVersion", "kind", "metadata", "spec")
def post_inference_service(namespace):
    cr = request.get_json()

    gvk = versions.inference_service_gvk()
    api.create_custom_rsrc(**gvk, data=cr, namespace=namespace)

    return api.success_response("message",
                                "InferenceService successfully created.")
