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

import hashlib
from pathlib import Path

import pytest

from kserve_storage.verification import get_single_artifact, sha256_file, verify_digest


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _write(path: Path, content: bytes) -> Path:
    """Write *content* to *path* and return the path."""
    path.write_bytes(content)
    return path


def _sha256_hex(content: bytes) -> str:
    """Return the bare hex SHA-256 digest of *content*."""
    return hashlib.sha256(content).hexdigest()


# ---------------------------------------------------------------------------
# sha256_file
# ---------------------------------------------------------------------------


class TestSha256File:
    def test_known_content(self, tmp_path):
        content = b"hello kserve"
        f = _write(tmp_path / "model.bin", content)
        result = sha256_file(f)
        assert result == f"sha256:{_sha256_hex(content)}"

    def test_empty_file(self, tmp_path):
        f = _write(tmp_path / "empty.bin", b"")
        result = sha256_file(f)
        expected = hashlib.sha256(b"").hexdigest()
        assert result == f"sha256:{expected}"

    def test_large_file_chunked(self, tmp_path):
        # Write a 3 MB file to exercise the chunked reading path.
        content = b"x" * (3 * 1024 * 1024)
        f = _write(tmp_path / "large.bin", content)
        result = sha256_file(f)
        assert result == f"sha256:{_sha256_hex(content)}"

    def test_return_format(self, tmp_path):
        f = _write(tmp_path / "model.bin", b"data")
        result = sha256_file(f)
        assert result.startswith("sha256:")
        hex_part = result[len("sha256:"):]
        # SHA-256 hex digest is always 64 characters
        assert len(hex_part) == 64
        assert all(c in "0123456789abcdef" for c in hex_part)


# ---------------------------------------------------------------------------
# verify_digest
# ---------------------------------------------------------------------------


class TestVerifyDigest:
    def test_matching_bare_hex(self, tmp_path):
        content = b"model weights"
        f = _write(tmp_path / "model.bin", content)
        expected = _sha256_hex(content)
        # Should not raise
        verify_digest(str(f), expected)

    def test_matching_with_sha256_prefix(self, tmp_path):
        content = b"model weights"
        f = _write(tmp_path / "model.bin", content)
        expected = f"sha256:{_sha256_hex(content)}"
        verify_digest(str(f), expected)

    def test_matching_uppercase_prefix(self, tmp_path):
        content = b"model weights"
        f = _write(tmp_path / "model.bin", content)
        expected = f"SHA256:{_sha256_hex(content).upper()}"
        verify_digest(str(f), expected)

    def test_mismatch_raises(self, tmp_path):
        content = b"original model"
        f = _write(tmp_path / "model.bin", content)
        wrong_digest = "a" * 64
        with pytest.raises(RuntimeError, match="Digest verification failed"):
            verify_digest(str(f), wrong_digest)

    def test_mismatch_error_contains_expected(self, tmp_path):
        content = b"model"
        f = _write(tmp_path / "model.bin", content)
        wrong = "b" * 64
        with pytest.raises(RuntimeError, match=wrong):
            verify_digest(str(f), wrong)

    def test_case_insensitive_comparison(self, tmp_path):
        content = b"case test"
        f = _write(tmp_path / "model.bin", content)
        expected_upper = _sha256_hex(content).upper()
        # Should match even though our sha256_file returns lowercase
        verify_digest(str(f), expected_upper)

    def test_tampered_file_fails(self, tmp_path):
        original = b"trusted model"
        tampered = b"tampered model"
        f = _write(tmp_path / "model.bin", tampered)
        correct_digest = _sha256_hex(original)
        with pytest.raises(RuntimeError, match="Digest verification failed"):
            verify_digest(str(f), correct_digest)


# ---------------------------------------------------------------------------
# get_single_artifact
# ---------------------------------------------------------------------------


class TestGetSingleArtifact:
    def test_returns_single_file_path(self, tmp_path):
        f = _write(tmp_path / "model.onnx", b"data")
        result = get_single_artifact(str(tmp_path))
        assert result == str(f)

    def test_raises_on_nonexistent_dir(self, tmp_path):
        missing = tmp_path / "does_not_exist"
        with pytest.raises(RuntimeError, match="does not exist"):
            get_single_artifact(str(missing))

    def test_raises_on_empty_dir(self, tmp_path):
        with pytest.raises(RuntimeError, match="Expected one downloaded artifact, found 0"):
            get_single_artifact(str(tmp_path))

    def test_raises_on_multiple_files(self, tmp_path):
        _write(tmp_path / "file1.bin", b"a")
        _write(tmp_path / "file2.bin", b"b")
        with pytest.raises(RuntimeError, match="Expected one downloaded artifact, found 2"):
            get_single_artifact(str(tmp_path))

    def test_raises_on_directory_artifact(self, tmp_path):
        subdir = tmp_path / "model_dir"
        subdir.mkdir()
        with pytest.raises(RuntimeError, match="found directory"):
            get_single_artifact(str(tmp_path))
