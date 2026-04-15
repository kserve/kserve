# Copyright 2025 The KServe Authors.
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

"""Post-download verification for model and LoRA adapter weights.

This module provides two verification layers that run after the storage
initializer downloads model artifacts:

1. **Signature verification** (via OpenSSF model-signing / Sigstore):
   Validates that the downloaded weights were signed by a trusted publisher
   and have not been tampered with in transit or at rest.

2. **Safety scanning** (via ModelScan / ProtectAI):
   Scans serialized model files for known malicious patterns, particularly
   pickle deserialization attacks that could execute arbitrary code when
   the model is loaded by the runtime.

Both layers are opt-in via environment variables and are designed to fail
closed: if verification or scanning fails, the storage initializer exits
with an error and the pod never starts.

Environment variables:
    KSERVE_VERIFY_SIGNATURES (str): Set to "true" to enable Sigstore
        signature verification on all downloaded artifacts.
    KSERVE_SCAN_MODELS (str): Set to "true" to enable ModelScan safety
        scanning on all downloaded artifacts.
"""

import logging
import os
import pathlib

logger = logging.getLogger(__name__)


def _str_to_bool(val: str) -> bool:
    return val.strip().lower() in ("true", "1", "yes")


def verify_signatures(dest_paths: list[str]) -> None:
    """Verify Sigstore signatures on downloaded model artifacts.

    Uses the OpenSSF model-signing library to verify that each downloaded
    artifact has a valid Sigstore signature. This ensures:
    - The artifact was published by a known identity (e.g., a HuggingFace user)
    - The artifact has not been modified since signing
    - The signature is logged in the Sigstore transparency log (Rekor)

    Args:
        dest_paths: List of local paths to verify.

    Raises:
        RuntimeError: If any artifact fails signature verification.
        ImportError: If the model-signing library is not installed.
    """
    try:
        from model_signing.signing import VerificationResult
        from model_signing.hashing import SHA256
        from model_signing.serializing import serialize_by_file_shard
        from model_signing.verifying import verify
    except ImportError:
        raise ImportError(
            "Signature verification requires the 'model-signing' package. "
            "Install it with: pip install model-signing"
        )

    for dest in dest_paths:
        if not dest or not os.path.exists(dest):
            continue

        dest_path = pathlib.Path(dest)
        logger.info("Verifying signature for: %s", dest_path)

        try:
            result: VerificationResult = verify(dest_path)
            if not result.verified:
                raise RuntimeError(
                    f"Signature verification FAILED for {dest_path}: {result.reason}"
                )
            logger.info(
                "Signature verification passed for %s (signer: %s)",
                dest_path,
                result.signer_identity or "unknown",
            )
        except FileNotFoundError:
            raise RuntimeError(
                f"No signature found for {dest_path}. "
                "Ensure the model was signed with Sigstore before publishing. "
                "See: https://github.com/sigstore/model-transparency"
            )


def scan_models(dest_paths: list[str]) -> None:
    """Scan downloaded model artifacts for known malicious patterns.

    Uses ModelScan to detect unsafe serialization patterns in model files,
    particularly pickle-based code execution attacks. This catches:
    - Arbitrary code execution via pickle __reduce__
    - Malicious Lambda layers in Keras/TensorFlow models
    - Unsafe operations in PyTorch models using pickle

    Note: ModelScan detects code execution attacks in serialized formats.
    It does NOT detect weight-poisoning backdoor attacks, which operate on
    the tensor values themselves. For weight-poisoning defense, signature
    verification (proving provenance) is the primary mitigation.

    Args:
        dest_paths: List of local paths to scan.

    Raises:
        RuntimeError: If any artifact contains unsafe patterns.
        ImportError: If the modelscan library is not installed.
    """
    try:
        from modelscan.modelscan import ModelScan
    except ImportError:
        raise ImportError(
            "Model scanning requires the 'modelscan' package. "
            "Install it with: pip install modelscan"
        )

    scanner = ModelScan()

    for dest in dest_paths:
        if not dest or not os.path.exists(dest):
            continue

        dest_path = pathlib.Path(dest)
        logger.info("Scanning for unsafe patterns: %s", dest_path)

        scan_result = scanner.scan(dest_path)

        if scan_result.issues:
            issue_details = "\n".join(
                f"  - {issue.severity}: {issue.description} ({issue.source})"
                for issue in scan_result.issues
            )
            raise RuntimeError(
                f"Safety scan FAILED for {dest_path}. "
                f"Found {len(scan_result.issues)} issue(s):\n{issue_details}"
            )

        logger.info(
            "Safety scan passed for %s (scanned %d files)",
            dest_path,
            scan_result.total_scanned,
        )


def run_post_download_verification(dest_paths: list[str]) -> None:
    """Run all enabled post-download verification steps.

    This is the main entry point called by the storage initializer
    entrypoint after all downloads complete. It checks environment
    variables to determine which verification steps to run.

    Args:
        dest_paths: List of local paths that were downloaded.

    Raises:
        RuntimeError: If any verification step fails.
    """
    verify_sigs = _str_to_bool(
        os.environ.get("KSERVE_VERIFY_SIGNATURES", "false")
    )
    scan_enabled = _str_to_bool(
        os.environ.get("KSERVE_SCAN_MODELS", "false")
    )

    if not verify_sigs and not scan_enabled:
        return

    logger.info(
        "Post-download verification enabled (signatures=%s, scan=%s)",
        verify_sigs,
        scan_enabled,
    )

    if verify_sigs:
        verify_signatures(dest_paths)

    if scan_enabled:
        scan_models(dest_paths)

    logger.info("All post-download verification passed.")
