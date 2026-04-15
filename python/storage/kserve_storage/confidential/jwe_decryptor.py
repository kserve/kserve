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

import logging
import os
from pathlib import Path

from jwcrypto import jwe, jwk

from .secret_resolver import SecretResolver

logger = logging.getLogger(__name__)

# JWE Compact Serialization starts with "eyJ" (base64url of '{"')
_JWE_HEADER_MAGIC = b"eyJ"
_JWE_EXTENSION = ".jwe"


class JWEDecryptor:
    """Decrypts JWE-encrypted model files using keys from a SecretResolver.

    Supports detection by file extension (``.jwe``) or by inspecting the first
    bytes of the file for the JWE Compact Serialization header.
    """

    def __init__(self, secret_resolver: SecretResolver, resource_id: str | None = None):
        """
        Args:
            secret_resolver: Resolver used to obtain decryption keys.
            resource_id: Default KBS resource ID for key resolution. If not
                provided, must be discoverable per-file or set via environment.
        """
        self._resolver = secret_resolver
        self._resource_id = resource_id

    @staticmethod
    def is_encrypted(file_path: str | Path) -> bool:
        """Check whether a file appears to be JWE-encrypted.

        Detection is based on the ``.jwe`` file extension or the presence of
        the JWE Compact Serialization header magic bytes.

        Args:
            file_path: Path to the file to check.

        Returns:
            True if the file is likely JWE-encrypted.
        """
        path = Path(file_path)

        if path.suffix.lower() == _JWE_EXTENSION:
            return True

        try:
            with open(path, "rb") as f:
                header = f.read(len(_JWE_HEADER_MAGIC))
                return header == _JWE_HEADER_MAGIC
        except OSError:
            return False

    def decrypt_file(
        self, file_path: str | Path, resource_id: str | None = None
    ) -> Path:
        """Decrypt a single JWE-encrypted file in place.

        The decrypted content replaces the encrypted file. If the file has a
        ``.jwe`` extension, it is removed from the output filename.

        Args:
            file_path: Path to the encrypted file.
            resource_id: KBS resource ID override for this file. Falls back to
                the instance default.

        Returns:
            Path to the decrypted file.

        Raises:
            SecretResolutionError: If the key cannot be resolved.
            ValueError: If no resource_id is available.
            jwe.InvalidJWEData: If the file is not valid JWE.
        """
        path = Path(file_path)
        rid = resource_id or self._resource_id
        if not rid:
            raise ValueError(
                f"No resource_id provided for decryption of {path}. "
                "Set resource_id on the decryptor or pass it to decrypt_file()."
            )

        logger.info("Decrypting %s with resource_id=%s", path, rid)

        key_bytes = self._resolver.resolve_key(rid)
        symmetric_key = jwk.JWK(kty="oct", k=jwk.base64url_encode(key_bytes))

        jwe_token = jwe.JWE()
        jwe_token.deserialize(path.read_text())
        jwe_token.decrypt(symmetric_key)
        plaintext = jwe_token.payload

        # Determine output path (strip .jwe extension if present)
        if path.suffix.lower() == _JWE_EXTENSION:
            output_path = path.with_suffix("")
        else:
            output_path = path

        output_path.write_bytes(plaintext)

        # Remove original if output path differs
        if output_path != path:
            path.unlink()

        logger.info("Decrypted %s -> %s", path, output_path)
        return output_path

    def decrypt_directory(
        self, dir_path: str | Path, resource_id: str | None = None
    ) -> list[Path]:
        """Walk a directory tree and decrypt all JWE-encrypted files.

        Args:
            dir_path: Root directory to scan.
            resource_id: KBS resource ID override. Falls back to the instance default.

        Returns:
            List of paths to decrypted files.

        Raises:
            SecretResolutionError: If the key cannot be resolved.
        """
        root = Path(dir_path)
        decrypted: list[Path] = []

        for dirpath, _, filenames in os.walk(root):
            for filename in filenames:
                file_path = Path(dirpath) / filename
                if self.is_encrypted(file_path):
                    output = self.decrypt_file(file_path, resource_id=resource_id)
                    decrypted.append(output)

        logger.info("Decrypted %d files in %s", len(decrypted), root)
        return decrypted
