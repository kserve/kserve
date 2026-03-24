"""Simple pipeline creating a kserve inference service"""

from kfp import dsl
import os


@dsl.pipeline(
    name="KServe Pipeline",
    description="A pipeline for creating a KServe inference service.",
)
def kserve_pipeline():
    from kfp import components

    action = os.getenv("ACTION")
    namespace = os.getenv("NAMESPACE")
    model_name = os.getenv("MODEL_NAME")
    model_uri = os.getenv("MODEL_URI")
    framework = os.getenv("FRAMEWORK")

    kserve_op = components.load_component_from_url(
        "https://raw.githubusercontent.com/hbelmiro/kfp_deploy_model_to_kserve_demo/refs/heads/main/component.yaml"
    )
    kserve_op(
        action=action,
        namespace=namespace,
        model_name=model_name,
        model_uri=model_uri,
        framework=framework,
    )
