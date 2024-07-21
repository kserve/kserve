import argparse
from argparse import ArgumentParser
from typing import List, Dict, Any

import tritonserver
from tritonserver import (
    InstanceGroupKind,
    ModelControlMode,
    RateLimitMode,
    RateLimiterResource,
)
from tritonserver._api._server import ModelLoadDeviceLimit


class TritonOptions:
    @staticmethod
    def add_cli_args(parser: ArgumentParser) -> ArgumentParser:
        triton_arg_parser = parser.add_argument_group(
            "triton server options", "Arguments for Triton server"
        )
        triton_arg_parser.add_argument(
            "--server_id",
            type=str,
            default="triton",
            help="Identifier for this server. Default value is 'triton'",
        )
        triton_arg_parser.add_argument(
            "--model_control_mode",
            choices=["none", "poll", "explicit"],
            default="none",
            type=TritonOptions.parse_model_control_mode,
            help="Specify the mode for model management. Options are 'none', 'poll' and 'explicit'. The default is "
            "'none'. For 'none', the server will load all models in the model repository(s) at startup and will "
            "not make any changes to the load models after that. For 'poll', the server will poll the model "
            "repository(s) to detect changes and will load/unload models based on those changes. The poll rate "
            "is controlled by 'repository_poll_secs'. For 'explicit', model load and unload is initiated by "
            "using the model control APIs, and only models specified with --load_model will be loaded at startup.",
        )
        triton_arg_parser.add_argument(
            "--load_model",
            type=str,
            nargs="*",
            default=[],  # Default is empty list to avoid NoneType
            help="Name of the model to be loaded on server startup. It may be specified multiple times to add "
            "multiple models. To load ALL models at startup, specify '*' as the model name with --load_model=* "
            "as the ONLY --load_model argument, this does not imply any pattern matching. Specifying "
            "--load-model=* in conjunction with another --load-model argument will result in error. Note that "
            "this option will only take effect if --model_control_mode=explicit is true.",
        )
        triton_arg_parser.add_argument(
            "--disable_auto_complete_config",
            action="store_true",
            help="If set, disables the triton and backends from auto completing model configuration files. Model "
            "configuration files must be provided and all required configuration settings must be specified.",
        )
        triton_arg_parser.add_argument(
            "--rate_limiter_mode",
            type=TritonOptions.parse_rate_limiter_mode,
            choices=["off", "execution_count"],
            default="off",
            help="Specify the mode for rate limiting. Options are 'execution_count' and 'off'. The default is 'off'. "
            "For 'execution_count', the server will determine the instance using configured priority and the "
            "number of time the instance has been used to run inference. The inference will finally be executed "
            "once the required resources are available. For 'off', the server will ignore any rate limiter "
            "config and run inference as soon as an instance is ready.",
        )
        triton_arg_parser.add_argument(
            "--rate_limiter_resource",
            nargs="*",
            type=TritonOptions.parse_rate_limiter_resource,
            default=[],  # Default is empty list to avoid NoneType
            help="The number of resources available to the server. The format of this flag is "
            "--rate_limiter_resource=<resource_name>:<count>:<device>. The <device> is optional and if not "
            "listed will be applied to every device. If the resource is specified as 'GLOBAL' in the model "
            "configuration the resource is considered shared among all the devices in the system. The <device> "
            "property is ignored for such resources. This flag can be specified multiple times to specify each "
            "resources and their availability. By default, the max across all instances that list the resource "
            "is selected as its availability. The values for this flag is case-insensitive.",
        )
        triton_arg_parser.add_argument(
            "--pinned_memory_pool_size",
            type=int,
            default=268435456,
            help="The total byte size that can be allocated as pinned system memory. If GPU support is enabled, "
            "the server will allocate pinned system memory to accelerate data transfer between host and devices "
            "until it exceeds the specified byte size. If 'numa-node' is configured via --host-policy, "
            "the pinned system memory of the pool size will be allocated on each numa node. This option will not "
            "affect the allocation conducted by the backend frameworks. Default is 256 MB.",
        )
        triton_arg_parser.add_argument(
            "--cuda_memory_pool_size",
            nargs="*",
            type=TritonOptions.parse_cuda_memory_pool_size,
            default={},  # Default is empty dict to avoid NoneType
            help="The total byte size that can be allocated as CUDA memory for the GPU device. If GPU support is "
            "enabled, the server will allocate CUDA memory to minimize data transfer between host and devices "
            "until it exceeds the specified byte size. This option will not affect the allocation conducted by "
            "the backend frameworks. The argument should be 2 integers separated by colons in the format <GPU "
            "device ID>:<pool byte size>. This option can be used multiple times, but only once per GPU device. "
            "Subsequent uses will overwrite previous uses for the same GPU device. Default is 64 MB.",
        )
        triton_arg_parser.add_argument(
            "--cache_config",
            type=TritonOptions.parse_cache_config,
            default={},  # Default is empty dict to avoid NoneType
            help="Specify a cache-specific configuration setting. The format of this flag is "
            "--cache_config=<cache_name>,<setting>=<value>. Where <cache_name> is the name of the cache, "
            "such as 'local' or 'redis'. Example: --cache-config=local,size=1048576 will configure a 'local' "
            "cache implementation with a fixed buffer pool of size 1048576 bytes.",
        )
        triton_arg_parser.add_argument(
            "--cache_directory",
            type=str,
            default="/opt/tritonserver/caches",
            help="The global directory searched for cache shared libraries. Default is '/opt/tritonserver/caches'. "
            "This directory is expected to contain a cache implementation as a shared library with the name "
            "'libtritoncache.so'.",
        )
        triton_arg_parser.add_argument(
            "--min_supported_compute_capability",
            type=float,
            default=6.0,
            help="The minimum supported CUDA compute capability. GPUs that don't support this compute capability will "
            "not be used by the server. Default is 6.0",
        )
        triton_arg_parser.add_argument(
            "--disable_exit_on_error",
            action="store_true",
            help="Prevents the inference server from shutting down automatically if an error occurs during "
            "initialization.",
        )
        triton_arg_parser.add_argument(
            "--disable_strict_readiness",
            action="store_true",
            help="If disabled /v2/health/ready endpoint indicates ready if server is responsive even if some/all "
            "models are unavailable. If enabled /v2/health/ready endpoint indicates ready if the server is "
            "responsive and all models are available.",
        )
        triton_arg_parser.add_argument(
            "--exit_timeout",
            type=int,
            default=30,
            help="Timeout (in seconds) when exiting to wait for in-flight inferences to finish. After the timeout "
            "expires the server exits even if inferences are still in flight. Default is 30 seconds.",
        )

        triton_arg_parser.add_argument(
            "--buffer_manager_thread_count",
            type=int,
            default=0,
            help="The number of threads used to accelerate copies and other operations required to manage input and "
            "output tensor contents. Default is 0.",
        )
        triton_arg_parser.add_argument(
            "--model_load_thread_count",
            type=int,
            default=4,
            help="The number of threads used to concurrently load models in model repositories. Default is 4.",
        )
        triton_arg_parser.add_argument(
            "--model_load_retry_count",
            type=int,
            default=0,
            help="The number of retry to load a model in model repositories. Default is 0.",
        )
        triton_arg_parser.add_argument(
            "--model_load_gpu_limit",
            nargs="*",
            type=TritonOptions.parse_model_load_gpu_limit,
            default=[],  # Default is empty list to avoid NoneType
            help="Specify the limit on GPU memory usage as a fraction. If model loading on the device is requested "
            "and the current memory usage exceeds the limit, the load will be rejected. If not specified, "
            "the limit will not be set. The argument should be 1 integer and 1 fraction separated by colons in "
            "the format <GPU device ID>:<fraction>. This option can be used multiple times, but only once per "
            "GPU device.",
        )
        triton_arg_parser.add_argument(
            "--model_namespacing",
            action="store_true",
            help="Enable model namespacing. If enabled, models with the same name can be served if "
            "they are in different namespace.",
        )
        triton_arg_parser.add_argument(
            "--disable_peer_access",
            action="store_true",
            help="Disable the peer access. Peer access could still be not enabled because the underlying system "
            "doesn't support it. The server will log a warning in this case.",
        )
        triton_arg_parser.add_argument(
            "--log_file",
            type=str,
            help="Set the name of the log output file. If specified, log outputs will be saved to this file. If not "
            "specified, log outputs will stream to the console.",
        )
        triton_arg_parser.add_argument(
            "--log_info", action="store_true", help="Enable info-level logging."
        )
        triton_arg_parser.add_argument(
            "--log_warn", action="store_true", help="Enable warning-level logging."
        )
        triton_arg_parser.add_argument(
            "--log_error", action="store_true", help="Enable error-level logging."
        )
        triton_arg_parser.add_argument(
            "--log_verbose",
            type=int,
            default=0,
            help="Set verbose logging level. Zero (0) disables verbose logging and values >= 1 enable verbose "
            "logging. Default is 0.",
        )
        triton_arg_parser.add_argument(
            "--log_format",
            choices=["default", "ISO8601"],
            type=TritonOptions.parse_log_format,
            default="default",
            help="Set the logging format. Options are 'default' and 'ISO8601'. The default is 'default'. For "
            "'default', the log severity (L) and timestamp will be logged as 'LMMDD hh:mm:ss.ssssss'. For "
            "'ISO8601', the log format will be 'YYYY-MM-DDThh:mm:ssZ L'.",
        )
        triton_arg_parser.add_argument(
            "--disable_metrics",
            action="store_true",
            help="Disable the server to provided prometheus metrics.",
        )
        triton_arg_parser.add_argument(
            "--disable_gpu_metrics", action="store_true", help="Disable GPU metrics."
        )
        triton_arg_parser.add_argument(
            "--disable_cpu_metrics", action="store_true", help="Disable CPU metrics."
        )
        triton_arg_parser.add_argument(
            "--metrics_interval",
            type=int,
            default=2000,
            help="Metrics will be collected once every <metrics-interval-ms> milliseconds. Default is 2000 "
            "milliseconds.",
        )
        triton_arg_parser.add_argument(
            "--metrics_config",
            nargs="*",
            type=TritonOptions.parse_metrics_config,
            default={},  # Default is empty dict to avoid NoneType
            help="Specify a metrics-specific configuration setting. The format of this flag is "
            "--metrics-config=<setting>=<value>. It can be specified multiple times.",
        )
        triton_arg_parser.add_argument(
            "--backend_directory",
            type=str,
            default="/opt/tritonserver/backends",
            help="The global directory searched for backend shared libraries. Default is '/opt/tritonserver/backends'.",
        )
        triton_arg_parser.add_argument(
            "--repo_agent_directory",
            type=str,
            default="/opt/tritonserver/repoagents",
            help="The global directory searched for repository agent shared libraries. Default is "
            "'/opt/tritonserver/repoagents'.",
        )
        triton_arg_parser.add_argument(
            "--backend_config",
            nargs="*",
            default={},  # Default is empty dict to avoid NoneType
            type=TritonOptions.parse_backend_config,
            help="Specify a backend-specific configuration setting. The format of this flag is "
            "--backend-config=<backend_name>,<setting>=<value> Where <backend_name> is the name of the backend, "
            "such as 'tensorrt'. It can be specified multiple times.",
        )
        triton_arg_parser.add_argument(
            "--host_policy",
            nargs="*",
            type=TritonOptions.parse_host_policy,
            default={},  # Default is empty dict to avoid NoneType
            help="Specify a host policy setting associated with a policy name. The format of this flag is "
            "--host-policy=<policy_name>,<setting>=<value>. Currently supported settings are 'numa-node', "
            "'cpu-cores'. Note that 'numa-node' setting will affect pinned memory pool behavior, "
            "see --pinned-memory-pool for more detail. It can be specified multiple times.",
        )
        return parser

    @classmethod
    def from_cli_args(cls, args: argparse.Namespace) -> tritonserver.Options:
        triton_options = tritonserver.Options(
            model_repository=args.model_dir,
            server_id=args.server_id,
            model_control_mode=args.model_control_mode,
            startup_models=args.load_model,
            strict_model_config=args.disable_auto_complete_config,
            rate_limiter_mode=args.rate_limiter_mode,
            rate_limiter_resources=args.rate_limiter_resource,
            pinned_memory_pool_size=args.pinned_memory_pool_size,
            cuda_memory_pool_sizes=args.cuda_memory_pool_size,
            cache_config=args.cache_config,
            cache_directory=args.cache_directory,
            min_supported_compute_capability=args.min_supported_compute_capability,
            exit_on_error=not args.disable_exit_on_error,
            strict_readiness=not args.disable_strict_readiness,
            exit_timeout=args.exit_timeout,
            buffer_manager_thread_count=args.buffer_manager_thread_count,
            model_load_thread_count=args.model_load_thread_count,
            model_load_retry_count=args.model_load_retry_count,
            model_load_device_limits=args.model_load_gpu_limit,
            model_namespacing=args.model_namespacing,
            enable_peer_access=not args.disable_peer_access,
            log_file=args.log_file,
            log_info=args.log_info,
            log_warn=args.log_warn,
            log_error=args.log_error,
            log_verbose=args.log_verbose,
            log_format=args.log_format,
            metrics=not args.disable_metrics,
            gpu_metrics=not args.disable_gpu_metrics,
            cpu_metrics=not args.disable_cpu_metrics,
            metrics_interval=args.metrics_interval,
            metrics_configuration=args.metrics_config,
            backend_directory=args.backend_directory,
            repo_agent_directory=args.repo_agent_directory,
            backend_configuration=args.backend_config,
            host_policies=args.host_policy,
        )
        return triton_options

    @staticmethod
    def parse_model_control_mode(mode: str) -> ModelControlMode:
        triton_model_control_mode = {
            "none": ModelControlMode.NONE,
            "poll": ModelControlMode.POLL,
            "explicit": ModelControlMode.EXPLICIT,
        }
        return triton_model_control_mode[mode]

    @staticmethod
    def parse_rate_limiter_mode(mode: str):
        triton_rate_limiter_mode = {
            "off": RateLimitMode.OFF,
            "execution_count": RateLimitMode.EXEC_COUNT,
        }
        return triton_rate_limiter_mode[mode]

    @staticmethod
    def parse_rate_limiter_resource(resources: List[str]):
        rate_limiter_resources = []
        for resource in resources:
            split_str = resource.split(":")
            if len(split_str) == 2:
                rate_limiter_resources.append(
                    RateLimiterResource(
                        name=split_str[0], count=int(split_str[1]), device=-1
                    )
                )
            elif len(split_str) == 3:
                rate_limiter_resources.append(
                    RateLimiterResource(
                        name=split_str[0],
                        count=int(split_str[1]),
                        device=int(split_str[2]),
                    )
                )
            else:
                raise ValueError(
                    f"--rate_limiter_resource option format is '<resource_name>:<count>:<device>' or "
                    f"'<resource_name>:<count>'. Got {resource}"
                )
        return rate_limiter_resources

    @staticmethod
    def parse_cuda_memory_pool_size(pool_sizes: List[str]) -> Dict[int, int]:
        cuda_memory_pool_sizes: Dict[int, int] = {}
        for pool_size in pool_sizes:
            split_str = pool_size.split(":")
            if len(split_str) != 2:
                raise ValueError(
                    f"--cuda_memory_pool_size option format is '<GPU device ID>:<pool byte size>'. Got {pool_size}"
                )
            cuda_memory_pool_sizes[int(split_str[0])] = int(split_str[1])
        return cuda_memory_pool_sizes

    @staticmethod
    def parse_cache_config(cache_config_str: str) -> Dict[str, Dict[str, Any]]:
        cache_config = {}
        # Format is "<cache_name>,<setting>=<value>" for specific config/settings
        cache_config_split = cache_config_str.split(",")
        cache_type = cache_config_split[0]
        if len(cache_config_split) != 2:
            raise ValueError(
                f"--cache_config option format is '<cache_name>,<setting>=<value>'. Got {cache_config}"
            )
        if len(cache_type) == 0:
            raise ValueError(
                f"No cache specified. --cache_config option format is <cache name>,<setting>=<value>. Got {cache_config}"
            )
        cache_config[cache_type] = {}
        cache_setting_split = cache_config_split[1].split("=")
        if len(cache_setting_split) != 2:
            raise ValueError(
                f"--cache_config option format is '<cache_name>,<setting>=<value>'. Got {cache_config}"
            )
        else:
            setting_name = cache_setting_split[0]
            setting_value = cache_setting_split[1]
            if len(setting_name) == 0 or len(setting_value) == 0:
                raise ValueError(
                    f"--cache_config option format is '<cache_name>,<setting>=<value>'. Got {cache_config}"
                )
            cache_config[cache_type][setting_name] = setting_value
        return cache_config

    @staticmethod
    def parse_model_load_gpu_limit(
        model_load_gpu_limit: List[str],
    ) -> List[ModelLoadDeviceLimit]:
        model_load_gpu_limits = []
        for limit in model_load_gpu_limit:
            split_str = limit.split(":")
            if len(split_str) != 2:
                raise ValueError(
                    f"--model_load_gpu_limit option format is '<GPU device ID>:<fraction>'. Got {limit}"
                )
            model_load_gpu_limits.append(
                ModelLoadDeviceLimit(
                    kind=InstanceGroupKind.GPU,
                    device=int(split_str[0]),
                    fraction=float(split_str[1]),
                )
            )
        return model_load_gpu_limits

    @staticmethod
    def parse_log_format(log_format: str) -> tritonserver.LogFormat:
        triton_log_format = {
            "default": tritonserver.LogFormat.DEFAULT,
            "ISO8601": tritonserver.LogFormat.ISO8601,
        }
        return triton_log_format[log_format]

    @staticmethod
    def parse_metrics_config(metrics_config: List[str]) -> Dict[str, Dict[str, str]]:
        metrics_config_dict = {}
        for config in metrics_config:
            # Format is "<setting>=<value>" for generic configs/settings
            setting_value_split = config.split("=")
            # Break section before "=" into substr to avoid matching commas in setting values.
            if len(setting_value_split[0].split(",")) != 1:
                # No name-specific configs currently supported, though it may be in
                # the future.
                raise ValueError(
                    f"--metrics_config option format is '<setting>=<value>'. Got {config}"
                )
            # No name-specific configs currently supported, though it may be in
            # the future. Map global configs to empty string like other configs for now.
            if "" in metrics_config_dict:
                metrics_config_dict[""][setting_value_split[0]] = setting_value_split[1]
            else:
                metrics_config_dict[""] = {
                    setting_value_split[0]: setting_value_split[1]
                }
        return metrics_config_dict

    @staticmethod
    def parse_backend_config(backend_configs: List[str]) -> Dict[str, Dict[str, str]]:
        backend_config_dict = {}
        for backend_config in backend_configs:
            #  Format is "<backend_name>,<setting>=<value>" for specific
            #  config/settings and "<setting>=<value>" for backend agnostic
            #  configs/settings
            backend_setting = backend_config
            if "," in backend_config:
                backend_config_split = backend_config.split(",")
                backend_name = backend_config_split[0]
                backend_setting = backend_config_split[1]
                if len(backend_name) == 0:
                    raise ValueError(
                        f"No backend specified. --backend_config option format is '<backend_name>,<setting>=<value>'. "
                        f"Got {backend_config}"
                    )
            else:
                # global backend config
                backend_name = ""
            backend_setting_split = backend_setting.split("=")
            if len(backend_setting_split) != 2:
                raise ValueError(
                    f"--backend_config option format is '<backend_name>,<setting>=<value>'. Got {backend_config}"
                )
            else:
                setting_name = backend_setting_split[0]
                setting_value = backend_setting_split[1]
                if len(setting_name) == 0 or len(setting_value) == 0:
                    raise ValueError(
                        f"--backend_config option format is '<backend_name>,<setting>=<value>'. Got {backend_config}"
                    )
                if backend_name in backend_config_dict:
                    backend_config_dict[backend_name][setting_name] = setting_value
                else:
                    backend_config_dict[backend_name] = {setting_name: setting_value}
        return backend_config_dict

    @staticmethod
    def parse_host_policy(host_policy: List[str]) -> Dict[str, Dict[str, str]]:
        host_policy_dict = {}
        for policy in host_policy:
            # Format is "<policy_name>,<setting>=<value>" for specific
            # config/settings
            policy_split = policy.split(",")
            policy_name = policy_split[0]
            if len(policy_name) == 0:
                raise ValueError(
                    f"No policy specified. --host_policy option format is '<policy_name>,<setting>=<value>'. Got {policy}"
                )
            policy_setting_split = policy_split[1].split("=")
            if len(policy_setting_split) != 2:
                raise ValueError(
                    f"--host_policy option format is '<policy_name>,<setting>=<value>'. Got {policy}"
                )
            else:
                setting_name = policy_setting_split[0]
                setting_value = policy_setting_split[1]
                if len(setting_name) == 0 or len(setting_value) == 0:
                    raise ValueError(
                        f"--host_policy option format is '<policy_name>,<setting>=<value>'. Got {policy}"
                    )
                if policy_name in host_policy_dict:
                    host_policy_dict[policy_name][setting_name] = setting_value
                else:
                    host_policy_dict[policy_name] = {setting_name: setting_value}
        return host_policy_dict
