import argparse
import json


def get_parsed_args():
    """
    Define and parse command-line arguments, including required and optional flags.
    """
    parser = argparse.ArgumentParser(description="Deploy and compile KServe pipeline")

    # required args
    parser.add_argument("--model_name", required=True, help="Name of the model")
    parser.add_argument(
        "--action",
        choices=["create", "update", "apply", "delete"],
        default="create",
        help=(
            "Action to execute on KServe. Options: create, update, apply, delete. "
            "Note: apply = update if resource exists, or create if not."
        ),
    )
    parser.add_argument("--namespace", help="Namespace to deploy the manifests")
    parser.add_argument("--model_uri", help="URI for the model")
    parser.add_argument("--framework", help="Framework used for the model")

    # Optional args
    parser.add_argument(
        "--runtime_version", default="latest", help="Runtime Version of ML Framework"
    )
    parser.add_argument(
        "--resource_requests",
        default='{"cpu": "0.5", "memory": "512Mi"}',
        help="CPU and Memory requests",
    )
    parser.add_argument(
        "--resource_limits",
        default='{"cpu": "1", "memory": "1Gi"}',
        help="CPU and Memory limits",
    )
    parser.add_argument(
        "--custom_model_spec",
        default="{}",
        help="Custom model runtime container spec in JSON",
    )
    parser.add_argument(
        "--autoscaling_target", default="0", help="Autoscaling Target Number"
    )
    parser.add_argument("--service_account", default="", help="ServiceAccount to use")
    parser.add_argument(
        "--enable_istio_sidecar",
        type=bool,
        default=True,
        help="Enable istio sidecar injection",
    )
    parser.add_argument(
        "--inferenceservice_yaml",
        default="{}",
        help="Raw InferenceService serialized YAML",
    )
    parser.add_argument(
        "--watch_timeout", default="300", help="Timeout for readiness (seconds)"
    )
    parser.add_argument("--min_replicas", default="-1", help="Minimum replicas")
    parser.add_argument("--max_replicas", default="-1", help="Maximum replicas")
    parser.add_argument(
        "--request_timeout", default="60", help="Component request timeout (seconds)"
    )
    parser.add_argument(
        "--enable_isvc_status",
        type=bool,
        default=True,
        help="Store inference service status as output",
    )
    parser.add_argument(
        "--canary_traffic_percent",
        default="100",
        help="Traffic split between candidate and last ready model",
    )

    args = parser.parse_args()

    # Enforce required args if action != delete
    if args.action != "delete":
        missing = []
        if not args.namespace:
            missing.append("--namespace")
        if not args.model_uri:
            missing.append("--model_uri")
        if not args.framework:
            missing.append("--framework")
        if missing:
            parser.error(
                f"The following arguments are required when action != delete: {', '.join(missing)}"
            )

    return args
