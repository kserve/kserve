from cli import get_parsed_args
from env_utils import set_env_vars
from deploy_utils import prepare_env_and_deploy, compile_pipeline
import os


def main():
    args = get_parsed_args()
    args_dict = vars(args)

    # Separate required args from optional flags
    required_keys = ["namespace", "action", "model_name", "model_uri", "framework"]
    required_args = {k: args_dict[k] for k in required_keys}

    # All other args (optional ones)
    optional_args = {k: v for k, v in args_dict.items() if k not in required_keys}

    set_env_vars(**required_args, **optional_args)

    if not os.getenv("SERVING_RUNTIME"):
        raise ValueError(
            "SERVING_RUNTIME environment variable is not set. Please set it to your ServingRuntime's file path."
        )

    prepare_env_and_deploy(args_dict)
    compile_pipeline(args_dict["action"])


if __name__ == "__main__":
    main()
