# Copyright 2024 The KServe Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import argparse
from argparse import ArgumentParser
from typing import Any, Optional, Union
from pathlib import Path

from kserve.logging import logger

try:
    import sglang
    from sglang.srt.server_args import ServerArgs as SGLangServerArgs
    _sglang = True
except ImportError:
    SGLangServerArgs = Any
    _sglang = False

from transformers import AutoConfig


def sglang_available() -> bool:
    return _sglang


def infer_sglang_supported_from_model_architecture(
    model_config_path: Union[Path, str],
    trust_remote_code: bool = False,
) -> bool:
    """
    Check if the model architecture is supported by SGLang.

    Args:
        model_config_path: Path to the model config
        trust_remote_code: Whether to trust remote code

    Returns:
        bool: Whether the model is supported by SGLang
    """
    if not _sglang:
        return False

    # SGLang supports most models that vLLM supports, but we should check specifically
    # This is a simplified check - in a real implementation, we would check against
    # SGLang's supported model list
    try:
        model_config = AutoConfig.from_pretrained(
            model_config_path, trust_remote_code=trust_remote_code
        )

        # Check if the model architecture is supported by SGLang
        # This is a simplified check - in a real implementation, we would check against
        # SGLang's supported model list
        supported_architectures = [
            "LlamaForCausalLM",
            "MistralForCausalLM",
            "FalconForCausalLM",
            "GPTNeoXForCausalLM",
            "GPT2LMHeadModel",
            "OPTForCausalLM",
            "BloomForCausalLM",
            "MPTForCausalLM",
            "PhiForCausalLM",
            "QWenLMHeadModel",
            "BaichuanForCausalLM",
            "ChatGLMModel",
            "InternLMForCausalLM",
            "YiForCausalLM",
        ]

        for architecture in model_config.architectures:
            if architecture not in supported_architectures:
                logger.info(f"Model architecture {architecture} not supported by SGLang")
                return False
        return True
    except Exception as e:
        logger.warning(f"Error checking SGLang model support: {e}")
        return False


def maybe_add_sglang_cli_parser(parser: ArgumentParser) -> ArgumentParser:
    """
    Add SGLang-specific CLI arguments to the parser if SGLang is available.

    Args:
        parser: The argument parser to add arguments to

    Returns:
        The updated argument parser
    """
    if not _sglang:
        return parser

    # Add SGLang-specific arguments
    # Memory management
    parser.add_argument(
        "--mem-fraction-static",
        type=float,
        help="The fraction of the memory used for static allocation (model weights and KV cache memory pool). Use a smaller value if you see out-of-memory errors.",
    )

    # Prefill configuration
    parser.add_argument(
        "--chunked-prefill-size",
        type=int,
        help="The maximum number of tokens in a chunk for the chunked prefill. Setting this to -1 means disabling chunked prefill.",
    )

    # Backend selection
    parser.add_argument(
        "--attention-backend",
        type=str,
        choices=["flashinfer", "triton", "torch_native", "fa3", "flashmla"],
        help="Choose the kernels for attention layers.",
    )

    parser.add_argument(
        "--sampling-backend",
        type=str,
        choices=["flashinfer", "pytorch"],
        help="Choose the kernels for sampling layers.",
    )

    parser.add_argument(
        "--grammar-backend",
        type=str,
        choices=["xgrammar", "outlines", "llguidance", "none"],
        help="Choose the backend for grammar-guided decoding.",
    )

    # Optimization options
    parser.add_argument(
        "--enable-torch-compile",
        action="store_true",
        help="Enable torch.compile for the model.",
    )

    parser.add_argument(
        "--stream-interval",
        type=int,
        help="The interval (or buffer size) for streaming in terms of the token length. A smaller value makes streaming smoother, while a larger value makes the throughput higher",
    )

    # Additional SGLang-specific parameters
    parser.add_argument(
        "--max-batch-size",
        type=int,
        help="Maximum batch size for SGLang inference.",
    )

    parser.add_argument(
        "--max-tokens-per-batch",
        type=int,
        help="Maximum number of tokens per batch for SGLang inference.",
    )

    parser.add_argument(
        "--max-context-len",
        type=int,
        help="Maximum context length for SGLang inference.",
    )

    parser.add_argument(
        "--enable-prefix-sharing",
        action="store_true",
        help="Enable prefix sharing for SGLang inference.",
    )

    parser.add_argument(
        "--enable-vllm-compatible",
        action="store_true",
        help="Enable vLLM compatibility mode for SGLang.",
    )

    parser.add_argument(
        "--enable-cuda-graph",
        action="store_true",
        help="Enable CUDA graph for SGLang inference.",
    )

    parser.add_argument(
        "--max-num-seqs",
        type=int,
        help="Maximum number of sequences for SGLang inference.",
    )

    return parser


def build_sglang_server_args(args) -> Optional["SGLangServerArgs"]:
    """
    Build SGLang server arguments from KServe arguments.

    Args:
        args: The KServe arguments

    Returns:
        SGLangServerArgs: The SGLang server arguments
    """
    if not _sglang:
        return None

    # Create SGLang server arguments with basic parameters
    sglang_args = SGLangServerArgs(
        model_path=args.model_id or args.model_dir,
        tokenizer_path=args.tokenizer or args.model_id or args.model_dir,
    )

    # Map standard Hugging Face parameters

    # Model and tokenizer configuration
    if hasattr(args, "trust_remote_code"):
        sglang_args.trust_remote_code = args.trust_remote_code

    if hasattr(args, "model_revision") and args.model_revision is not None:
        sglang_args.revision = args.model_revision

    if hasattr(args, "tokenizer_revision") and args.tokenizer_revision is not None:
        sglang_args.tokenizer_revision = args.tokenizer_revision

    # Data type configuration
    if hasattr(args, "dtype") and args.dtype is not None:
        # Map dtype formats
        dtype_mapping = {
            "auto": "auto",
            "float16": "float16",
            "half": "float16",
            "float32": "float32",
            "float": "float32",
            "bfloat16": "bfloat16"
        }
        sglang_args.dtype = dtype_mapping.get(args.dtype, "auto")

    # Context length
    if hasattr(args, "max_model_len") and args.max_model_len is not None:
        sglang_args.context_length = args.max_model_len
    elif hasattr(args, "max_length") and args.max_length is not None:
        sglang_args.context_length = args.max_length

    # Task configuration
    if hasattr(args, "task") and args.task is not None:
        # SGLang doesn't directly use task, but we can configure based on task
        # For now, we just log the task
        logger.info(f"Task set to {args.task}, configuring SGLang accordingly")

    # Tokenizer configuration
    if hasattr(args, "tokenizer_mode") and args.tokenizer_mode is not None:
        # SGLang uses different tokenizer configuration
        # For now, we just log the tokenizer mode
        logger.info(f"Tokenizer mode set to {args.tokenizer_mode}")

    # Special tokens handling
    if hasattr(args, "disable_special_tokens") and args.disable_special_tokens:
        # Store this for later use in SGLangModel
        sglang_args.add_special_tokens = False

    # Case sensitivity
    if hasattr(args, "disable_lower_case") and args.disable_lower_case:
        # Store this for later use in SGLangModel
        sglang_args.do_lower_case = False

    # SGLang-specific parameters
    # Memory management
    if hasattr(args, "mem_fraction_static") and args.mem_fraction_static is not None:
        sglang_args.mem_fraction_static = args.mem_fraction_static

    # Prefill configuration
    if hasattr(args, "chunked_prefill_size") and args.chunked_prefill_size is not None:
        sglang_args.chunked_prefill_size = args.chunked_prefill_size

    # Backend selection
    if hasattr(args, "attention_backend") and args.attention_backend is not None:
        sglang_args.attention_backend = args.attention_backend

    if hasattr(args, "sampling_backend") and args.sampling_backend is not None:
        sglang_args.sampling_backend = args.sampling_backend

    if hasattr(args, "grammar_backend") and args.grammar_backend is not None:
        sglang_args.grammar_backend = args.grammar_backend

    # Optimization options
    if hasattr(args, "enable_torch_compile") and args.enable_torch_compile:
        sglang_args.enable_torch_compile = True

    if hasattr(args, "stream_interval") and args.stream_interval is not None:
        sglang_args.stream_interval = args.stream_interval

    # Additional SGLang-specific parameters
    if hasattr(args, "max_batch_size") and args.max_batch_size is not None:
        sglang_args.max_batch_size = args.max_batch_size

    if hasattr(args, "max_tokens_per_batch") and args.max_tokens_per_batch is not None:
        sglang_args.max_tokens_per_batch = args.max_tokens_per_batch

    if hasattr(args, "max_context_len") and args.max_context_len is not None:
        sglang_args.max_context_len = args.max_context_len

    if hasattr(args, "enable_prefix_sharing") and args.enable_prefix_sharing:
        sglang_args.enable_prefix_sharing = True

    if hasattr(args, "enable_vllm_compatible") and args.enable_vllm_compatible:
        sglang_args.enable_vllm_compatible = True

    if hasattr(args, "enable_cuda_graph") and args.enable_cuda_graph:
        sglang_args.enable_cuda_graph = True

    if hasattr(args, "max_num_seqs") and args.max_num_seqs is not None:
        sglang_args.max_num_seqs = args.max_num_seqs

    return sglang_args


def validate_sglang_args(args: argparse.Namespace):
    """
    Validate SGLang-specific arguments.

    Args:
        args: The parsed arguments
    """
    if not _sglang:
        return

    if args.backend == "sglang" and not sglang_available():
        raise RuntimeError("Backend is set to 'sglang' but SGLang is not available")

    # Validate that the model is supported by SGLang
    if args.backend == "sglang":
        model_id_or_path = args.model_id or args.model_dir
        if not infer_sglang_supported_from_model_architecture(
            model_id_or_path, trust_remote_code=args.trust_remote_code
        ):
            raise RuntimeError(
                f"Model {model_id_or_path} is not supported by SGLang. "
                "Please use a supported model or change the backend."
            )
