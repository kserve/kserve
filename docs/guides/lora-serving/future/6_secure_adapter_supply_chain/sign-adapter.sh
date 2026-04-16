#!/usr/bin/env bash
# sign-adapter.sh — Download a LoRA adapter from HuggingFace, sign it with
# Sigstore (OpenSSF model-signing), verify the signature, and optionally
# push the signed adapter to a new HuggingFace repo.
#
# Prerequisites:
#   pip install model-signing huggingface-hub cryptography
#   pip install modelscan  # optional, for safety scanning
#
# Usage:
#   # Sign and keep locally
#   ./sign-adapter.sh cimendev/kubernetes-qa-qwen2.5-7b-lora
#
#   # Sign and push to a new HF repo
#   ./sign-adapter.sh cimendev/kubernetes-qa-qwen2.5-7b-lora my-org/k8s-lora-signed
#
# What it does:
#   1. Downloads the adapter from HuggingFace
#   2. Runs ModelScan to check for pickle deserialization attacks
#   3. Signs the adapter with Sigstore (keyless / OIDC, opens browser)
#   4. Extracts signer identity from the certificate and verifies
#   5. Prints the KSERVE_SIGNER_IDENTITY / KSERVE_SIGNER_ISSUER values
#   6. Optionally pushes the signed adapter (with model.sig) to HuggingFace

set -euo pipefail

SOURCE_REPO="${1:?Usage: $0 <source-hf-repo> [dest-hf-repo]}"
DEST_REPO="${2:-}"

WORK_DIR="$(mktemp -d)"
ADAPTER_DIR="${WORK_DIR}/adapter"

cleanup() {
    if [ -d "${WORK_DIR}" ]; then
        echo ""
        echo "Working directory preserved at: ${WORK_DIR}"
        echo "  Adapter files:  ${ADAPTER_DIR}/"
        echo "  Signature:      ${ADAPTER_DIR}/model.sig"
        echo ""
        echo "To clean up: rm -rf ${WORK_DIR}"
    fi
}
trap cleanup EXIT

echo "============================================"
echo " LoRA Adapter Signing Pipeline"
echo "============================================"
echo ""
echo "Source:  ${SOURCE_REPO}"
echo "Dest:   ${DEST_REPO:-<local only>}"
echo "Workdir: ${WORK_DIR}"
echo ""

# -----------------------------------------------
# Step 1: Download
# -----------------------------------------------
echo "--- Step 1: Downloading adapter from HuggingFace ---"
echo ""

python3 -c "
from huggingface_hub import snapshot_download
snapshot_download(
    repo_id='${SOURCE_REPO}',
    local_dir='${ADAPTER_DIR}',
    local_dir_use_symlinks=False,
)
print('Download complete.')
"

echo ""
echo "Downloaded files:"
ls -lh "${ADAPTER_DIR}/"
echo ""

# -----------------------------------------------
# Step 2: Safety scan
# -----------------------------------------------
echo "--- Step 2: Scanning for unsafe patterns ---"
echo ""

if command -v modelscan &> /dev/null; then
    echo "Running ModelScan..."
    modelscan scan -p "${ADAPTER_DIR}/" || {
        echo ""
        echo "WARNING: ModelScan found issues. Review the output above."
        echo "Continue anyway? (Ctrl+C to abort)"
        read -r
    }
    echo ""
else
    echo "ModelScan not installed (pip install modelscan). Skipping safety scan."
    echo ""
fi

# -----------------------------------------------
# Step 3: Sign with Sigstore
# -----------------------------------------------
echo "--- Step 3: Signing adapter with Sigstore ---"
echo ""
echo "This will open a browser for OIDC authentication."
echo "Your identity will be recorded in the Sigstore transparency log."
echo ""

python3 -c "
import model_signing.signing
from pathlib import Path

adapter_path = Path('${ADAPTER_DIR}')
sig_path = adapter_path / 'model.sig'

print(f'Signing: {adapter_path}')
print(f'Output:  {sig_path}')
print()

model_signing.signing.sign(adapter_path, sig_path)

print()
print(f'Signature written to: {sig_path}')
print(f'Signature size: {sig_path.stat().st_size} bytes')
"

echo ""

# -----------------------------------------------
# Step 4: Extract identity and verify
# -----------------------------------------------
echo "--- Step 4: Verifying signature ---"
echo ""

python3 - "${ADAPTER_DIR}" << 'PYEOF' || {
    echo ""
    echo "ERROR: Signature verification FAILED."
    exit 1
}
import json
import base64
import sys
from pathlib import Path

try:
    from cryptography.x509 import load_der_x509_certificate
except ImportError:
    print("WARNING: 'cryptography' package not installed.")
    print("  Cannot extract signer identity or verify.")
    print("  Install with: pip install cryptography")
    sys.exit(0)

import model_signing.verifying

adapter_path = Path(sys.argv[1])
sig_path = adapter_path / "model.sig"

print(f"Verifying: {adapter_path}")
print(f"Signature: {sig_path}")
print()

# Parse the Sigstore bundle to extract the signer identity and OIDC issuer
# from the signing certificate.
bundle = json.loads(sig_path.read_text())
certs = bundle["verificationMaterial"]["x509CertificateChain"]["certificates"]
cert_der = base64.b64decode(certs[0]["rawBytes"])
cert = load_der_x509_certificate(cert_der)

identity = None
issuer = None

for ext in cert.extensions:
    # SubjectAlternativeName — contains the signer email or URI
    if ext.oid.dotted_string == "2.5.29.17":
        for name in ext.value:
            identity = name.value
            break
    # Sigstore OIDC issuer OID
    if ext.oid.dotted_string == "1.3.6.1.4.1.57264.1.1":
        issuer = ext.value.value.decode()

if not identity or not issuer:
    print("WARNING: Could not extract identity/issuer from certificate.")
    print("  Signature file was created but could not be self-verified.")
    sys.exit(0)

print(f"Signer identity: {identity}")
print(f"OIDC issuer:     {issuer}")
print()

# Verify the signature matches the adapter contents
model_signing.verifying.Config().use_sigstore_verifier(
    identity=identity,
    oidc_issuer=issuer,
).verify(adapter_path, sig_path)

print("Signature verification PASSED.")
print()
print("=" * 50)
print("Use these values in your LLMInferenceService:")
print(f"  KSERVE_SIGNER_IDENTITY: {identity}")
print(f"  KSERVE_SIGNER_ISSUER:   {issuer}")
print("=" * 50)
PYEOF

# -----------------------------------------------
# Step 5: Push to HuggingFace (optional)
# -----------------------------------------------
if [ -n "${DEST_REPO}" ]; then
    echo "--- Step 5: Pushing signed adapter to HuggingFace ---"
    echo ""
    echo "Destination: ${DEST_REPO}"
    echo ""

    python3 -c "
from huggingface_hub import HfApi

api = HfApi()

try:
    api.create_repo(repo_id='${DEST_REPO}', exist_ok=True)
    print(f'Repo ready: ${DEST_REPO}')
except Exception as e:
    print(f'Note: {e}')

api.upload_folder(
    folder_path='${ADAPTER_DIR}',
    repo_id='${DEST_REPO}',
    commit_message='Upload signed LoRA adapter (Sigstore model-signing)',
)
print(f'Pushed to: https://huggingface.co/${DEST_REPO}')
"

    echo ""
    echo "============================================"
    echo " Done! Signed adapter pushed to HuggingFace"
    echo "============================================"
    echo ""
    echo "  Repo: https://huggingface.co/${DEST_REPO}"
    echo "  Signature: model.sig (Sigstore bundle)"
    echo ""
    echo "  To use in LLMInferenceService:"
    echo "    lora:"
    echo "      adapters:"
    echo "        - name: my-adapter"
    echo "          uri: hf://${DEST_REPO}"
    echo ""
else
    echo "============================================"
    echo " Done! Adapter signed locally"
    echo "============================================"
    echo ""
    echo "  Adapter: ${ADAPTER_DIR}/"
    echo "  Signature: ${ADAPTER_DIR}/model.sig"
    echo ""
    echo "  To push manually:"
    echo "    huggingface-cli upload <your-org>/<repo-name> ${ADAPTER_DIR}/"
    echo ""
fi
