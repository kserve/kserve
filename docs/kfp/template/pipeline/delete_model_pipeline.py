"""Simple pipeline deleting a kserve inference service"""

from kfp import dsl
import os


@dsl.pipeline(
    name="Kserve Pipeline",
    description="A pipeline for deleting a KServe inference service",
)
def kserve_pipeline():
    from kfp import components

    action = os.getenv("ACTION")
    model_name = os.getenv("MODEL_NAME")
    namespace = os.getenv("NAMESPACE")

    kserve_op = components.load_component_from_url(
        "https://raw.githubusercontent.com/hbelmiro/kfp_deploy_model_to_kserve_demo/refs/heads/main/component.yaml"
    )

    kserve_op(action=action, model_name=model_name, namespace=namespace)
