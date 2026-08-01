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

import os
from unittest.mock import MagicMock, patch

import pytest
from jwcrypto import jwe, jwk

from kserve_storage.confidential import (
    CDHSecretResolver,
    JWEDecryptor,
    SecretResolutionError,
    SecretResolver,
)


# --- Helpers ---


class StubSecretResolver(SecretResolver):
    """A test resolver that returns a fixed key."""

    def __init__(self, key: bytes):
        self._key = key

    def resolve_key(self, resource_id: str) -> bytes:
        return self._key


class FailingSecretResolver(SecretResolver):
    """A test resolver that always fails."""

    def resolve_key(self, resource_id: str) -> bytes:
        raise SecretResolutionError("resolver failure")


def _encrypt_jwe(plaintext: bytes, key_bytes: bytes) -> str:
    """Create a JWE Compact Serialization token using A256KW + A256GCM."""
    symmetric_key = jwk.JWK(kty="oct", k=jwk.base64url_encode(key_bytes))
    token = jwe.JWE(
        plaintext,
        protected={"alg": "A256KW", "enc": "A256GCM"},
    )
    token.add_recipient(symmetric_key)
    return token.serialize(compact=True)


# --- SecretResolver interface ---


class TestSecretResolverContract:
    def test_stub_resolver_returns_key(self):
        key = b"0123456789abcdef0123456789abcdef"
        resolver = StubSecretResolver(key)
        assert resolver.resolve_key("kbs:///repo/type/tag") == key

    def test_failing_resolver_raises(self):
        resolver = FailingSecretResolver()
        with pytest.raises(SecretResolutionError, match="resolver failure"):
            resolver.resolve_key("kbs:///repo/type/tag")

    def test_cannot_instantiate_abstract_class(self):
        with pytest.raises(TypeError):
            SecretResolver()


# --- CDHSecretResolver ---


class TestCDHSecretResolver:
    def test_default_cdh_addr(self, monkeypatch):
        monkeypatch.delenv("CDH_ADDR", raising=False)
        resolver = CDHSecretResolver()
        assert resolver._cdh_addr == "http://127.0.0.1:8006"

    def test_explicit_cdh_addr(self):
        resolver = CDHSecretResolver(cdh_addr="http://localhost:9999")
        assert resolver._cdh_addr == "http://localhost:9999"

    def test_invalid_resource_id_raises(self):
        resolver = CDHSecretResolver()
        with pytest.raises(SecretResolutionError, match="Invalid resource ID"):
            resolver.resolve_key("invalid-id")

    @patch("kserve_storage.confidential.cdh_client.requests.get")
    def test_valid_resource_id_format(self, mock_get):
        key_data = b"0123456789abcdef0123456789abcdef"
        mock_response = MagicMock()
        mock_response.content = key_data
        mock_response.raise_for_status = MagicMock()
        mock_get.return_value = mock_response

        resolver = CDHSecretResolver()
        result = resolver.resolve_key("kbs:///default/key/model-key")

        assert result == key_data
        mock_get.assert_called_once_with(
            "http://127.0.0.1:8006/cdh/resource/default/key/model-key",
            timeout=30,
        )


# --- JWEDecryptor ---


class TestJWEDecryptorDetection:
    def test_detect_by_extension(self, tmp_path):
        jwe_file = tmp_path / "model.bin.jwe"
        jwe_file.write_text("dummy content")
        assert JWEDecryptor.is_encrypted(jwe_file) is True

    def test_detect_by_header_magic(self, tmp_path):
        key_bytes = os.urandom(32)
        token = _encrypt_jwe(b"secret data", key_bytes)
        encrypted_file = tmp_path / "model.bin"
        encrypted_file.write_text(token)
        assert JWEDecryptor.is_encrypted(encrypted_file) is True

    def test_not_encrypted_plain_file(self, tmp_path):
        plain_file = tmp_path / "model.bin"
        plain_file.write_bytes(b"plain model data")
        assert JWEDecryptor.is_encrypted(plain_file) is False

    def test_not_encrypted_nonexistent(self, tmp_path):
        assert JWEDecryptor.is_encrypted(tmp_path / "nonexistent") is False


class TestJWEDecryptorRoundTrip:
    def test_decrypt_file_with_jwe_extension(self, tmp_path):
        plaintext = b"model weights data here"
        key_bytes = os.urandom(32)
        token = _encrypt_jwe(plaintext, key_bytes)

        encrypted_file = tmp_path / "model.bin.jwe"
        encrypted_file.write_text(token)

        resolver = StubSecretResolver(key_bytes)
        decryptor = JWEDecryptor(resolver, resource_id="kbs:///repo/type/tag")
        output = decryptor.decrypt_file(encrypted_file)

        assert output == tmp_path / "model.bin"
        assert output.read_bytes() == plaintext
        assert not encrypted_file.exists()

    def test_decrypt_file_without_jwe_extension(self, tmp_path):
        plaintext = b"model weights data here"
        key_bytes = os.urandom(32)
        token = _encrypt_jwe(plaintext, key_bytes)

        encrypted_file = tmp_path / "model.bin"
        encrypted_file.write_text(token)

        resolver = StubSecretResolver(key_bytes)
        decryptor = JWEDecryptor(resolver, resource_id="kbs:///repo/type/tag")
        output = decryptor.decrypt_file(encrypted_file)

        assert output == encrypted_file
        assert output.read_bytes() == plaintext

    def test_decrypt_file_no_resource_id_raises(self, tmp_path):
        key_bytes = os.urandom(32)
        token = _encrypt_jwe(b"data", key_bytes)
        encrypted_file = tmp_path / "model.bin.jwe"
        encrypted_file.write_text(token)

        resolver = StubSecretResolver(key_bytes)
        decryptor = JWEDecryptor(resolver)
        with pytest.raises(ValueError, match="No resource_id"):
            decryptor.decrypt_file(encrypted_file)


class TestJWEDecryptorDirectory:
    def test_decrypt_directory(self, tmp_path):
        plaintext_a = b"weights file a"
        plaintext_b = b"weights file b"
        key_bytes = os.urandom(32)

        # Encrypted file with .jwe extension
        (tmp_path / "model_a.bin.jwe").write_text(_encrypt_jwe(plaintext_a, key_bytes))

        # Encrypted file detected by header (no .jwe extension)
        (tmp_path / "model_b.bin").write_text(_encrypt_jwe(plaintext_b, key_bytes))

        # Plain file (not encrypted)
        (tmp_path / "config.json").write_text('{"key": "value"}')

        # Nested directory with encrypted file
        subdir = tmp_path / "subdir"
        subdir.mkdir()
        (subdir / "adapter.jwe").write_text(_encrypt_jwe(b"adapter data", key_bytes))

        resolver = StubSecretResolver(key_bytes)
        decryptor = JWEDecryptor(resolver, resource_id="kbs:///repo/type/tag")
        decrypted = decryptor.decrypt_directory(tmp_path)

        assert len(decrypted) == 3
        assert (tmp_path / "model_a.bin").read_bytes() == plaintext_a
        assert (tmp_path / "model_b.bin").read_bytes() == plaintext_b
        assert (subdir / "adapter").read_bytes() == b"adapter data"
        assert (tmp_path / "config.json").read_text() == '{"key": "value"}'

    def test_decrypt_directory_empty(self, tmp_path):
        resolver = StubSecretResolver(os.urandom(32))
        decryptor = JWEDecryptor(resolver, resource_id="kbs:///repo/type/tag")
        decrypted = decryptor.decrypt_directory(tmp_path)
        assert decrypted == []
