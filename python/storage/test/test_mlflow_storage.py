# Copyright 2021 The KServe Authors.
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
import unittest.mock as mock

import pytest

from kserve_storage import Storage


class TestDownloadMlflow:
    """Tests for Storage._download_mlflow() method."""

    @mock.patch.dict(os.environ, {}, clear=True)
    def test_missing_tracking_uri_raises_error(self, tmp_path):
        """Should raise ValueError when MLFLOW_TRACKING_URI is not set."""
        uri = "mlflow://models:/my-model/1"
        out_dir = str(tmp_path)

        with pytest.raises(ValueError, match="Cannot find MLFlow tracking Uri"):
            Storage._download_mlflow(uri, out_dir)

    @mock.patch("mlflow.artifacts.download_artifacts")
    @mock.patch("mlflow.set_tracking_uri")
    @mock.patch.dict(
        os.environ,
        {"MLFLOW_TRACKING_URI": "http://mlflow.example.com"},
        clear=True,
    )
    def test_successful_download(
        self, mock_set_tracking_uri, mock_download_artifacts, tmp_path
    ):
        """Should successfully download artifacts when configuration is valid."""
        uri = "mlflow://models:/my-model/1"
        out_dir = str(tmp_path)

        result = Storage._download_mlflow(uri, out_dir)

        mock_set_tracking_uri.assert_called_once_with("http://mlflow.example.com")
        mock_download_artifacts.assert_called_once_with(
            artifact_uri="models:/my-model/1", dst_path=out_dir
        )
        assert result == out_dir

    @mock.patch("mlflow.set_tracking_uri")
    @mock.patch.dict(
        os.environ,
        {"MLFLOW_TRACKING_URI": "http://mlflow.example.com"},
        clear=True,
    )
    def test_empty_model_uri_raises_error(self, mock_set_tracking_uri, tmp_path):
        """Should raise ValueError when model uri is empty (just 'mlflow://')."""
        uri = "mlflow://"
        out_dir = str(tmp_path)

        with pytest.raises(ValueError, match="Model uri cannot be empty"):
            Storage._download_mlflow(uri, out_dir)

    @mock.patch("mlflow.set_tracking_uri")
    @mock.patch.dict(
        os.environ,
        {
            "MLFLOW_TRACKING_URI": "http://mlflow.example.com",
            "MLFLOW_TRACKING_USERNAME": "user",
            "MLFLOW_TRACKING_PASSWORD": "password",
            "MLFLOW_TRACKING_TOKEN": "token123",
        },
        clear=True,
    )
    def test_token_with_credentials_raises_error(self, mock_set_tracking_uri, tmp_path):
        """Should raise ValueError when token is set along with username/password."""
        uri = "mlflow://models:/my-model/1"
        out_dir = str(tmp_path)

        with pytest.raises(
            ValueError, match="Tracking Token cannot be set with Username/Password"
        ):
            Storage._download_mlflow(uri, out_dir)

    @mock.patch("mlflow.artifacts.download_artifacts")
    @mock.patch("mlflow.set_tracking_uri")
    @mock.patch.dict(
        os.environ,
        {"MLFLOW_TRACKING_URI": "http://mlflow.example.com"},
        clear=True,
    )
    def test_mlflow_exception_raises_runtime_error(
        self, mock_set_tracking_uri, mock_download_artifacts, tmp_path
    ):
        """Should raise RuntimeError when MLflow download fails."""
        from mlflow.exceptions import MlflowException

        mock_download_artifacts.side_effect = MlflowException("Download failed")
        uri = "mlflow://models:/my-model/1"
        out_dir = str(tmp_path)

        with pytest.raises(RuntimeError, match="Failed to download model from MLFlow"):
            Storage._download_mlflow(uri, out_dir)

    @mock.patch("mlflow.artifacts.download_artifacts")
    @mock.patch("mlflow.set_tracking_uri")
    @mock.patch.dict(
        os.environ,
        {
            "MLFLOW_TRACKING_URI": "http://mlflow.example.com",
            "MLFLOW_TRACKING_USERNAME": "user",
            "MLFLOW_TRACKING_PASSWORD": "password",
        },
        clear=True,
    )
    def test_download_with_username_password(
        self, mock_set_tracking_uri, mock_download_artifacts, tmp_path
    ):
        """Should successfully download with username/password authentication."""
        uri = "mlflow://models:/my-model/Production"
        out_dir = str(tmp_path)

        result = Storage._download_mlflow(uri, out_dir)

        mock_set_tracking_uri.assert_called_once_with("http://mlflow.example.com")
        mock_download_artifacts.assert_called_once_with(
            artifact_uri="models:/my-model/Production", dst_path=out_dir
        )
        assert result == out_dir

    @mock.patch("mlflow.artifacts.download_artifacts")
    @mock.patch("mlflow.set_tracking_uri")
    @mock.patch.dict(
        os.environ,
        {
            "MLFLOW_TRACKING_URI": "http://mlflow.example.com",
            "MLFLOW_TRACKING_TOKEN": "token123",
        },
        clear=True,
    )
    def test_download_with_token(
        self, mock_set_tracking_uri, mock_download_artifacts, tmp_path
    ):
        """Should successfully download with token authentication."""
        uri = "mlflow://runs:/abc123/artifacts/model"
        out_dir = str(tmp_path)

        result = Storage._download_mlflow(uri, out_dir)

        mock_set_tracking_uri.assert_called_once_with("http://mlflow.example.com")
        mock_download_artifacts.assert_called_once_with(
            artifact_uri="runs:/abc123/artifacts/model", dst_path=out_dir
        )
        assert result == out_dir
