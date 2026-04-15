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
with an error and the pod never starts — weights are never transferred to
the main container.

Environment variables:
    KSERVE_VERIFY_SIGNATURES (str): Set to "true" to enable Sigstore
        signature verification on all downloaded artifacts.
    KSERVE_SIGNER_IDENTITY (str): Required when signature verification is
        enabled. The expected signer identity (e.g., "user@example.com"
        or a GitHub Actions workflow URI).
    KSERVE_SIGNER_ISSUER (str): Required when signature verification is
        enabled. The OIDC issuer for the signer identity (e.g.,
        "https://accounts.google.com", "https://github.com/login/oauth").
    KSERVE_SCAN_MODELS (str): Set to "true" to enable ModelScan safety
        scanning on all downloaded artifacts.
"""

import logging
import os
import pathlib

logger = logging.getLogger(__name__)

SIGNATURE_FILENAME = "model.sig"


def _str_to_bool(val: str) -> bool:
    return val.strip().lower() in ("true", "1", "yes")


def verify_signatures(dest_paths: list[str]) -> None:
    """Verify Sigstore signatures on downloaded model artifacts.

    Uses the OpenSSF model-signing library to verify that each downloaded
    artifact has a valid Sigstore signature. This ensures:
    - The artifact was published by a known identity
    - The artifact has not been modified since signing
    - The signature is logged in the Sigstore transparency log (Rekor)

    Requires KSERVE_SIGNER_IDENTITY and KSERVE_SIGNER_ISSUER environment
    variables to be set, so the verifier knows which identity to trust.

    Args:
        dest_paths: List of local paths to verify.

    Raises:
        RuntimeError: If any artifact fails signature verification or
            if required environment variables are missing.
        ImportError: If the model-signing library is not installed.
    """
    try:
        import model_signing.verifying
    except ImportError:
        raise ImportError(
            "Signature verification requires the 'model-signing' package. "
            "Install it with: pip install model-signing"
        )

    signer_identity = os.environ.get("KSERVE_SIGNER_IDENTITY", "")
    signer_issuer = os.environ.get("KSERVE_SIGNER_ISSUER", "")

    if not signer_identity or not signer_issuer:
        raise RuntimeError(
            "Signature verification requires KSERVE_SIGNER_IDENTITY and "
            "KSERVE_SIGNER_ISSUER environment variables. "
            "KSERVE_SIGNER_IDENTITY is the expected signer email or URI. "
            "KSERVE_SIGNER_ISSUER is the OIDC issuer URL "
            "(e.g., https://accounts.google.com)."
        )

    verifier = model_signing.verifying.Config().use_sigstore_verifier(
        identity=signer_identity,
        oidc_issuer=signer_issuer,
    )

    for dest in dest_paths:
        if not dest or not os.path.exists(dest):
            continue

        dest_path = pathlib.Path(dest)
        sig_path = dest_path / SIGNATURE_FILENAME

        if not sig_path.exists():
            raise RuntimeError(
                f"No signature found for {dest_path} "
                f"(expected {sig_path}). "
                "Ensure the model was signed with Sigstore before publishing. "
                "See: https://github.com/sigstore/model-transparency"
            )

        logger.info("Verifying signature for: %s", dest_path)

        try:
            verifier.verify(
                model_path=dest_path,
                signature_path=sig_path,
            )
        except ValueError as e:
            raise RuntimeError(
                f"Signature verification FAILED for {dest_path}: {e}"
            )

        logger.info(
            "Signature verification passed for %s (identity: %s, issuer: %s)",
            dest_path,
            signer_identity,
            signer_issuer,
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
    entrypoint after all downloads complete but BEFORE the init container
    exits. If any check fails, the init container exits with an error and
    the downloaded weights are never made available to the main container.

    Verification order:
    1. Signature verification (if KSERVE_VERIFY_SIGNATURES=true)
    2. Safety scanning (if KSERVE_SCAN_MODELS=true)

    Both checks fail closed — any failure prevents the pod from starting.

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
