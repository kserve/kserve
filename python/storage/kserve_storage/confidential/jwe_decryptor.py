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

import base64
import binascii
import json
import logging
import os
import tempfile
from dataclasses import dataclass
from pathlib import Path

from cryptography.exceptions import InvalidTag
from cryptography.hazmat.primitives.ciphers import Cipher, algorithms, modes
from cryptography.hazmat.primitives.keywrap import InvalidUnwrap, aes_key_unwrap
from jwcrypto import jwe, jwk

from .secret_resolver import SecretResolver

logger = logging.getLogger(__name__)

# JWE Compact Serialization starts with "eyJ" (base64url of '{"')
_JWE_HEADER_MAGIC = b"eyJ"
_JWE_EXTENSION = ".jwe"

# Compact JWE: BASE64URL(header).BASE64URL(ek).BASE64URL(iv).BASE64URL(ct).BASE64URL(tag)
# Header, wrapped CEK, and IV are small; only the ciphertext grows with the model.
_MAX_HEADER_REGION = 64 * 1024
_TAG_TAIL_REGION = 256
# Encoded ciphertext is read in 1 MiB chunks so peak RAM stays O(chunk), not O(file).
_STREAM_CHUNK_SIZE = 1024 * 1024
_GCM_IV_LEN = 12
_GCM_TAG_LEN = 16
_KW_KEY_LEN = {"A128KW": 16, "A192KW": 24, "A256KW": 32}
_GCM_KEY_LEN = {"A128GCM": 16, "A192GCM": 24, "A256GCM": 32}
_B64_WHITESPACE = b" \t\r\n"


@dataclass(frozen=True)
class _CompactJWE:
    """Byte offsets and decoded metadata for a compact JWE on disk."""

    protected_b64: bytes
    header: dict
    encrypted_key: bytes
    iv: bytes
    tag: bytes
    ciphertext_start: int
    ciphertext_end: int


class _StreamingB64UrlDecoder:
    """Incrementally decode unpadded base64url without buffering the full input."""

    def __init__(self) -> None:
        self._buf = b""

    def feed(self, data: bytes) -> bytes:
        if not data:
            return b""
        data = data.translate(None, _B64_WHITESPACE)
        if self._buf:
            data = self._buf + data
        n = len(data) & ~3
        self._buf = data[n:]
        if n == 0:
            return b""
        return _b64url_decode(data[:n])

    def finalize(self) -> bytes:
        leftover = self._buf
        self._buf = b""
        if not leftover:
            return b""
        return _b64url_decode(leftover)


def _b64url_decode(data: bytes) -> bytes:
    """Decode unpadded or padded base64url. Raises InvalidJWEData on corrupt input."""
    data = data.translate(None, _B64_WHITESPACE)
    pad = (-len(data)) % 4
    try:
        return base64.b64decode(
            data.replace(b"-", b"+").replace(b"_", b"/") + b"=" * pad,
            validate=True,
        )
    except (binascii.Error, ValueError) as exc:
        raise jwe.InvalidJWEData(
            "Invalid base64url in JWE compact serialization"
        ) from exc


def _parse_compact_jwe(path: Path) -> _CompactJWE:
    """Locate compact JWE parts on disk without loading the ciphertext."""
    size = path.stat().st_size
    if size < 8:
        raise jwe.InvalidJWEData(f"{path} is too small to be a compact JWE")

    with path.open("rb") as f:
        prefix = f.read(min(_MAX_HEADER_REGION, size))
        if prefix.startswith(b"{"):
            raise jwe.InvalidJWEData(
                f"{path} uses JWE JSON serialization; only compact "
                "serialization is supported for streaming decryption"
            )
        dots: list[int] = []
        for i, byte in enumerate(prefix):
            if byte == 46:  # '.'
                dots.append(i)
                if len(dots) == 3:
                    break
        if len(dots) < 3:
            raise jwe.InvalidJWEData(
                f"{path} is not compact JWE "
                "(expected header.encrypted_key.iv.ciphertext.tag)"
            )

        protected_b64 = prefix[: dots[0]]
        ek_b64 = prefix[dots[0] + 1 : dots[1]]
        iv_b64 = prefix[dots[1] + 1 : dots[2]]
        ciphertext_start = dots[2] + 1

        tail_len = min(_TAG_TAIL_REGION, size)
        f.seek(size - tail_len)
        tail = f.read()
        trimmed = tail.rstrip(_B64_WHITESPACE)
        rel_dot = trimmed.rfind(b".")
        if rel_dot < 0:
            raise jwe.InvalidJWEData(
                f"{path} is not compact JWE (missing authentication tag)"
            )
        last_dot = (size - tail_len) + rel_dot
        if last_dot < ciphertext_start:
            raise jwe.InvalidJWEData(f"{path} is not compact JWE")
        tag_b64 = trimmed[rel_dot + 1 :]

    try:
        header = json.loads(_b64url_decode(protected_b64))
    except json.JSONDecodeError as exc:
        raise jwe.InvalidJWEData(f"{path} has an invalid JWE protected header") from exc
    if not isinstance(header, dict):
        raise jwe.InvalidJWEData(f"{path} has an invalid JWE protected header")

    return _CompactJWE(
        protected_b64=protected_b64,
        header=header,
        encrypted_key=_b64url_decode(ek_b64),
        iv=_b64url_decode(iv_b64),
        tag=_b64url_decode(tag_b64),
        ciphertext_start=ciphertext_start,
        ciphertext_end=last_dot,
    )


def _oct_key_bytes(symmetric_key: jwk.JWK) -> bytes:
    exported = symmetric_key.export(as_dict=True)
    if exported.get("kty") != "oct" or "k" not in exported:
        raise ValueError(
            "Only symmetric oct JWKs are supported for streaming JWE decryption"
        )
    return _b64url_decode(exported["k"].encode("ascii"))


def _unwrap_cek(alg: str, enc: str, kek: bytes, encrypted_key: bytes) -> bytes:
    if enc not in _GCM_KEY_LEN:
        raise jwe.InvalidJWEData(
            f"Unsupported JWE enc algorithm {enc!r}. "
            "Streaming decryption supports A128GCM, A192GCM, and A256GCM."
        )
    expected_cek = _GCM_KEY_LEN[enc]

    if alg == "dir":
        if encrypted_key:
            raise jwe.InvalidJWEData(
                "Direct ('dir') JWE must have an empty encrypted key"
            )
        if len(kek) != expected_cek:
            raise jwe.InvalidJWEData(
                f"Direct key length {len(kek)} does not match {enc}"
            )
        return kek

    if alg in _KW_KEY_LEN:
        if len(kek) != _KW_KEY_LEN[alg]:
            raise jwe.InvalidJWEData(
                f"Key wrap key length {len(kek)} does not match {alg}"
            )
        try:
            cek = aes_key_unwrap(kek, encrypted_key)
        except InvalidUnwrap as exc:
            raise jwe.InvalidJWEData(
                "Failed to unwrap JWE content encryption key"
            ) from exc
        if len(cek) != expected_cek:
            raise jwe.InvalidJWEData(
                f"Unwrapped CEK length {len(cek)} does not match {enc}"
            )
        return cek

    raise jwe.InvalidJWEData(
        f"Unsupported JWE alg {alg!r}. "
        "Streaming decryption supports dir, A128KW, A192KW, and A256KW."
    )


def _stream_aes_gcm_decrypt(
    path: Path, compact: _CompactJWE, cek: bytes, dest: Path
) -> None:
    """Decrypt compact JWE ciphertext from disk to dest; authenticate before return.

    Plaintext is written incrementally. The caller must treat dest as untrusted
    until this function returns successfully (GCM tag verified in finalize()).
    """
    if len(compact.iv) != _GCM_IV_LEN:
        raise jwe.InvalidJWEData(
            f"JWE IV length {len(compact.iv)} is invalid for AES-GCM (expected 12)"
        )
    if len(compact.tag) != _GCM_TAG_LEN:
        raise jwe.InvalidJWEData(
            f"JWE tag length {len(compact.tag)} is invalid for AES-GCM (expected 16)"
        )

    decryptor = Cipher(
        algorithms.AES(cek),
        modes.GCM(compact.iv, compact.tag),
    ).decryptor()
    # RFC 7516: AAD is ASCII(BASE64URL(UTF8(JWE Protected Header)))
    decryptor.authenticate_additional_data(compact.protected_b64)

    decoder = _StreamingB64UrlDecoder()
    remaining = compact.ciphertext_end - compact.ciphertext_start

    with path.open("rb") as inf, dest.open("wb") as out:
        inf.seek(compact.ciphertext_start)
        while remaining > 0:
            encoded = inf.read(min(_STREAM_CHUNK_SIZE, remaining))
            if not encoded:
                raise jwe.InvalidJWEData(f"Unexpected EOF in JWE ciphertext of {path}")
            remaining -= len(encoded)
            decoded = decoder.feed(encoded)
            if decoded:
                out.write(decryptor.update(decoded))
        tail = decoder.finalize()
        if tail:
            out.write(decryptor.update(tail))
        try:
            final = decryptor.finalize()
        except InvalidTag as exc:
            raise jwe.InvalidJWEData(
                "JWE authentication tag verification failed"
            ) from exc
        if final:
            out.write(final)
        out.flush()
        os.fsync(out.fileno())


class JWEDecryptor:
    """Decrypts JWE-encrypted model files using keys from a SecretResolver.

    Supports detection by file extension (``.jwe``) or by inspecting the first
    bytes of the file for the JWE Compact Serialization header.

    Compact AES-GCM JWEs are decrypted in constant memory: ciphertext is
    streamed from disk, and plaintext is written to a temporary file that is
    published only after the authentication tag verifies.
    """

    def __init__(self, secret_resolver: SecretResolver, resource_id: str | None = None):
        """
        Args:
            secret_resolver: Resolver used to obtain decryption keys.
            resource_id: Default KBS resource ID for key resolution. Must be
                provided either here or per-call to decrypt_file/decrypt_directory.
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

        Large compact JWEs are streamed so peak memory does not grow with
        model size. The encrypted source is left untouched until decryption
        and authentication succeed.

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

        symmetric_key = self._load_symmetric_key(rid)
        compact = _parse_compact_jwe(path)
        header = compact.header
        if header.get("zip"):
            raise jwe.InvalidJWEData(
                "JWE compression (zip) is not supported for streaming decryption"
            )
        if header.get("crit"):
            raise jwe.InvalidJWEData("JWE crit headers are not supported")

        alg = header.get("alg")
        enc = header.get("enc")
        if not alg or not enc:
            raise jwe.InvalidJWEData("JWE protected header must include alg and enc")

        kek = _oct_key_bytes(symmetric_key)
        cek = _unwrap_cek(alg, enc, kek, compact.encrypted_key)

        if path.suffix.lower() == _JWE_EXTENSION:
            output_path = path.with_suffix("")
        else:
            output_path = path

        tmp_path: Path | None = None
        try:
            fd, tmp_name = tempfile.mkstemp(
                prefix=".jwe-decrypt.",
                suffix=".tmp",
                dir=str(output_path.parent),
            )
            os.close(fd)
            tmp_path = Path(tmp_name)
            logger.info(
                "Streaming JWE decrypt of %s (alg=%s enc=%s) -> %s",
                path,
                alg,
                enc,
                output_path,
            )
            _stream_aes_gcm_decrypt(path, compact, cek, tmp_path)
            os.replace(tmp_path, output_path)
            tmp_path = None
        finally:
            if tmp_path is not None:
                tmp_path.unlink(missing_ok=True)

        if output_path != path:
            path.unlink()

        logger.info("Decrypted %s -> %s", path, output_path)
        return output_path

    def _load_symmetric_key(self, resource_id: str) -> jwk.JWK:
        key_bytes = self._resolver.resolve_key(resource_id)
        try:
            # the kubernetes secret likely returns a utf-8 encoded value.
            # so try to decode utf-8.
            key_str = key_bytes.decode("utf-8")
            try:
                # if kubernetes secret was created from the full encryption key file,
                # the value should be the full json.
                return jwk.JWK.from_json(key_str)
            except (ValueError, jwk.InvalidJWKValue):
                # Not a JWK object - treat as raw key value
                return jwk.JWK(kty="oct", k=key_str)
        except UnicodeDecodeError:
            # if we fail to decode, then the key is likely raw-bytes and should be encoded.
            return jwk.JWK(kty="oct", k=jwe.base64url_encode(key_bytes))

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
