import os

from kubeflow.kubeflow.crud_backend import api, helpers, logging

log = logging.getLogger(__name__)

KNATIVE_REVISION_LABEL = "serving.knative.dev/revision"
FILE_ABS_PATH = os.path.abspath(os.path.dirname(__file__))

INFERENCESERVICE_TEMPLATE_YAML = os.path.join(
    FILE_ABS_PATH, "yaml", "inference_service_template.yaml")


def load_inference_service_template(**kwargs):
    """
    kwargs: the parameters to be replaced in the yaml

    Reads the yaml for the web app's custom resource, replaces the variables
    and returns it as a python dict.
    """
    return helpers.load_param_yaml(INFERENCESERVICE_TEMPLATE_YAML, **kwargs)


# helper functions for accessing the logs of an InferenceService
def get_inference_service_pods(svc, components=[]):
    """
    Return a dictionary with (endpoint, component) keys,
    i.e. ("default", "predictor") and a list of pod names as values
    """
    namespace = svc["metadata"]["namespace"]

    # dictionary{revisionName: (endpoint, component)}
    revisions_dict = get_components_revisions_dict(components, svc)

    if len(revisions_dict.keys()) == 0:
        return {}

    pods = api.list_pods(namespace, auth=False).items
    component_pods_dict = {}
    for pod in pods:
        for revision in revisions_dict:
            if KNATIVE_REVISION_LABEL not in pod.metadata.labels:
                continue

            if pod.metadata.labels[KNATIVE_REVISION_LABEL] != revision:
                continue

            component = revisions_dict[revision]
            curr_pod_names = component_pods_dict.get(component, [])
            curr_pod_names.append(pod.metadata.name)
            component_pods_dict[component] = curr_pod_names

    if len(component_pods_dict.keys()) == 0:
        log.info("No pods are found for inference service: %s",
                 svc["metadata"]["name"])

    return component_pods_dict


# FIXME(elikatsis,kimwnasptd): Change the logic of this function according to
# https://github.com/arrikto/dev/issues/867
def get_components_revisions_dict(components, svc):
    """
    Return a dictionary{revisionId: component}
    """
    status = svc["status"]
    revisions_dict = {}

    for component in components:
        if "components" not in status:
            log.info("Component '%s' not in inference service '%s'",
                     component, svc["metadata"]["name"])
            continue

        if component not in status["components"]:
            log.info("Component '%s' not in inference service '%s'",
                     component, svc["metadata"]["name"])
            continue

        if "latestReadyRevision" in status["components"][component]:
            revision = status["components"][component]["latestReadyRevision"]

            revisions_dict[revision] = component

    if len(revisions_dict.keys()) == 0:
        log.info(
            "No revisions found for the inference service's components: %s",
            svc["metadata"]["name"],
        )

    return revisions_dict
