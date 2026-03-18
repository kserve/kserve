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

"""CLI tool for decrypting JWE-encrypted model artifacts.

Usage:
    kserve-decrypt --source-dir /mnt/models --resource-id kbs:///repo/type/tag
    kserve-decrypt --source-dir /mnt/models  # uses CONFIDENTIAL_RESOURCE_ID env var
"""

import argparse
import logging
import os
import sys

from .jwe_decryptor import JWEDecryptor
from .cdh_client import CDHSecretResolver

logger = logging.getLogger(__name__)


def main():
    parser = argparse.ArgumentParser(
        description="Decrypt JWE-encrypted model artifacts using CDH key resolution."
    )
    parser.add_argument(
        "--source-dir",
        required=True,
        help="Directory containing encrypted model files to decrypt.",
    )
    parser.add_argument(
        "--resource-id",
        default=os.environ.get("CONFIDENTIAL_RESOURCE_ID", ""),
        help="KBS resource ID (kbs:///<repo>/<type>/<tag>). "
        "Defaults to CONFIDENTIAL_RESOURCE_ID env var.",
    )
    args = parser.parse_args()

    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s - %(name)s - %(levelname)s - %(message)s",
    )

    resource_id = args.resource_id or None

    try:
        resolver = CDHSecretResolver()
        decryptor = JWEDecryptor(resolver, resource_id=resource_id)
        decrypted = decryptor.decrypt_directory(args.source_dir, resource_id=resource_id)
        logger.info("Decrypted %d files in %s", len(decrypted), args.source_dir)
    except Exception as e:
        logger.error("Decryption failed: %s", e)
        sys.exit(1)


if __name__ == "__main__":
    main()
