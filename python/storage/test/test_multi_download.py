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
import tempfile
import unittest.mock as mock
import pytest

from kserve_storage import Storage

STORAGE_MODULE = "kserve_storage.kserve_storage"


class TestMultiDownload:
    """Tests for Storage.download_files() multi-download functionality."""

    def test_download_multiple_local_files(self):
        """Test downloading multiple local files to different directories."""
        with tempfile.TemporaryDirectory() as temp_dir:
            # Create source files
            src1 = os.path.join(temp_dir, "source1")
            src2 = os.path.join(temp_dir, "source2")
            os.makedirs(src1)
            os.makedirs(src2)

            # Create dummy files
            with open(os.path.join(src1, "model1.pth"), "w") as f:
                f.write("model1")
            with open(os.path.join(src2, "model2.pth"), "w") as f:
                f.write("model2")

            # Download to separate destinations
            dest1 = os.path.join(temp_dir, "dest1")
            dest2 = os.path.join(temp_dir, "dest2")

            results = Storage.download_files([src1, src2], [dest1, dest2])

            assert len(results) == 2
            assert results[0] == dest1
            assert results[1] == dest2
            assert os.path.exists(os.path.join(dest1, "model1.pth"))
            assert os.path.exists(os.path.join(dest2, "model2.pth"))

    def test_download_creates_output_directories(self):
        """Test that download_files creates output directories if they don't exist."""
        with tempfile.TemporaryDirectory() as temp_dir:
            # Create source files
            src1 = os.path.join(temp_dir, "source1")
            os.makedirs(src1)
            with open(os.path.join(src1, "model.pth"), "w") as f:
                f.write("model")

            # Destination doesn't exist yet
            dest1 = os.path.join(temp_dir, "nested", "dest1")
            assert not os.path.exists(dest1)

            Storage.download_files([src1], [dest1])

            # Should create the directory
            assert os.path.exists(dest1)
            assert os.path.exists(os.path.join(dest1, "model.pth"))

    @mock.patch(STORAGE_MODULE + ".Storage.download")
    @mock.patch("os.makedirs")
    def test_download_calls_download_for_each_uri(self, mock_makedirs, mock_download):
        """Test that download_files calls Storage.download for each URI/path pair."""
        mock_download.side_effect = lambda uri, out, **kwargs: out

        uris = ["file:///path1", "file:///path2", "file:///path3"]
        out_dirs = ["/dest1", "/dest2", "/dest3"]

        results = Storage.download_files(uris, out_dirs)

        assert len(results) == 3
        assert mock_download.call_count == 3

        # Verify each call
        expected_calls = [
            mock.call(
                "file:///path1", "/dest1", allow_patterns=None, ignore_patterns=None
            ),
            mock.call(
                "file:///path2", "/dest2", allow_patterns=None, ignore_patterns=None
            ),
            mock.call(
                "file:///path3", "/dest3", allow_patterns=None, ignore_patterns=None
            ),
        ]
        mock_download.assert_has_calls(expected_calls)

    @mock.patch(STORAGE_MODULE + ".Storage.download")
    @mock.patch("os.makedirs")
    def test_download_files_with_allow_patterns(self, mock_makedirs, mock_download):
        """Test that allow_patterns are passed to each download call."""
        mock_download.side_effect = lambda uri, out, **kwargs: out

        allow_patterns = ["*.safetensors", "*.json"]
        uris = ["file:///model1", "file:///model2"]
        out_dirs = ["/dest1", "/dest2"]

        Storage.download_files(uris, out_dirs, allow_patterns=allow_patterns)

        # Verify allow_patterns passed to each call
        for call in mock_download.call_args_list:
            assert call[1]["allow_patterns"] == allow_patterns

    @mock.patch(STORAGE_MODULE + ".Storage.download")
    @mock.patch("os.makedirs")
    def test_download_files_with_ignore_patterns(self, mock_makedirs, mock_download):
        """Test that ignore_patterns are passed to each download call."""
        mock_download.side_effect = lambda uri, out, **kwargs: out

        ignore_patterns = ["*.bin", "*.gguf"]
        uris = ["file:///model1", "file:///model2"]
        out_dirs = ["/dest1", "/dest2"]

        Storage.download_files(uris, out_dirs, ignore_patterns=ignore_patterns)

        # Verify ignore_patterns passed to each call
        for call in mock_download.call_args_list:
            assert call[1]["ignore_patterns"] == ignore_patterns

    @mock.patch(STORAGE_MODULE + ".Storage.download")
    @mock.patch("os.makedirs")
    def test_download_mixed_uri_schemes(self, mock_makedirs, mock_download):
        """Test downloading from mixed URI schemes (local, http, s3, hf)."""
        mock_download.side_effect = lambda uri, out, **kwargs: out

        # Mix of different URI schemes
        uris = [
            "file:///local/path",
            "https://example.com/model.tar.gz",
            "s3://bucket/model",
            "hf://org/repo",
        ]
        out_dirs = ["/dest1", "/dest2", "/dest3", "/dest4"]

        results = Storage.download_files(uris, out_dirs)

        assert len(results) == 4
        assert mock_download.call_count == 4

    def test_download_empty_lists(self):
        """Test that empty lists don't cause errors."""
        results = Storage.download_files([], [])
        assert results == []

    @mock.patch(STORAGE_MODULE + ".Storage.download")
    @mock.patch("os.makedirs")
    def test_download_mismatched_list_lengths_raises_error(
        self, mock_makedirs, mock_download
    ):
        """Test that mismatched URI and out_dir list lengths raise an error."""
        with pytest.raises(ValueError):
            Storage.download_files(
                ["file:///path1", "file:///path2"],
                ["/dest1"],  # Mismatched length
            )

    @mock.patch(STORAGE_MODULE + ".Storage.download")
    @mock.patch("os.makedirs")
    def test_download_partial_failure_raises_exception(
        self, mock_makedirs, mock_download
    ):
        """Test that if one download fails, the exception is raised."""
        # First download succeeds, second fails
        mock_download.side_effect = ["/dest1", RuntimeError("Download failed")]

        with pytest.raises(RuntimeError, match="Download failed"):
            Storage.download_files(
                ["file:///path1", "file:///path2"], ["/dest1", "/dest2"]
            )

    def test_download_with_none_output_dir(self):
        """Test that None output directory uses default download behavior."""
        with tempfile.TemporaryDirectory() as temp_dir:
            src = os.path.join(temp_dir, "source")
            os.makedirs(src)
            with open(os.path.join(src, "model.pth"), "w") as f:
                f.write("model")

            # None should use default download behavior (returns source path)
            results = Storage.download_files([src], [None])

            assert len(results) == 1
            # With None, local files return the source path
            assert src in results[0]

    @mock.patch(STORAGE_MODULE + ".Storage.download")
    @mock.patch("os.makedirs")
    def test_download_returns_paths_in_order(self, mock_makedirs, mock_download):
        """Test that download_files returns paths in the same order as input."""
        # Return different paths to verify ordering
        mock_download.side_effect = ["/result1", "/result2", "/result3"]

        uris = ["file:///path1", "file:///path2", "file:///path3"]
        out_dirs = ["/dest1", "/dest2", "/dest3"]

        results = Storage.download_files(uris, out_dirs)

        assert results == ["/result1", "/result2", "/result3"]
