import os


def set_env_vars(namespace, action, model_name, model_uri, framework, **kwargs):
    print("Setting environment variables:")
    base_vars = {
        "NAMESPACE": namespace,
        "ACTION": action,
        "MODEL_NAME": model_name,
        "MODEL_URI": model_uri,
        "FRAMEWORK": framework,
    }

    # Map of arg name to ENV VAR name
    extras = {
        "runtime_version": "RUNTIME_VERSION",
        "resource_requests": "RESOURCE_REQUESTS",
        "resource_limits": "RESOURCE_LIMITS",
        "custom_model_spec": "CUSTOM_MODEL_SPEC",
        "autoscaling_target": "AUTOSCALING_TARGET",
        "service_account": "SERVICE_ACCOUNT",
        "enable_istio_sidecar": "ENABLE_ISTIO_SIDECAR",
        "inferenceservice_yaml": "INFERENCESERVICE_YAML",
        "watch_timeout": "WATCH_TIMEOUT",
        "min_replicas": "MIN_REPLICAS",
        "max_replicas": "MAX_REPLICAS",
        "request_timeout": "REQUEST_TIMEOUT",
        "enable_isvc_status": "ENABLE_ISVC_STATUS",
        "canary_traffic_percent": "CANARY_TRAFFIC_PERCENT",
    }

    for env_var, value in base_vars.items():
        os.environ[env_var] = str(value)
        print(f"  {env_var}={value}")

    for arg_key, env_var in extras.items():
        value = kwargs.get(arg_key)
        if value is not None:
            if isinstance(value, bool):
                value = str(value).lower()
            os.environ[env_var] = str(value)
            print(f"  {env_var}={value}")
