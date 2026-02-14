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
import json
import pytest
import botocore
import tempfile
import unittest.mock as mock

from botocore.client import Config
from botocore import UNSIGNED
from kserve_storage import Storage


class MockPool:
    """
    Mock multiprocessing.Pool that executes tasks synchronously in the same process.
    This allows boto3 mocks to work properly in tests.
    """

    def __init__(self, processes=None, initializer=None, initargs=(), **kwargs):
        self.processes = processes
        self.initializer = initializer
        self.initargs = initargs
        # Call initializer immediately if provided (simulating worker process initialization)
        if self.initializer:
            self.initializer(*self.initargs)

    def __enter__(self):
        return self

    def __exit__(self, exc_type, exc_val, exc_tb):
        pass

    def map(self, func, iterable):
        """Execute tasks synchronously instead of in parallel processes"""
        return [func(item) for item in iterable]


# Mock multiprocessing.Pool globally for all S3 tests
mock.patch("kserve_storage.kserve_storage.multiprocessing.Pool", MockPool).start()


def create_mock_obj(path):
    mock_obj = mock.MagicMock()
    mock_obj.key = path
    mock_obj.size = 1024  # Non-zero size to avoid being skipped
    mock_obj.is_dir = False
    return mock_obj


def create_mock_boto3_bucket(mock_storage, paths):
    mock_s3_resource = mock.MagicMock()
    mock_s3_bucket = mock.MagicMock()
    mock_s3_bucket.objects.filter.return_value = [create_mock_obj(p) for p in paths]

    mock_s3_resource.Bucket.return_value = mock_s3_bucket
    mock_storage.return_value = mock_s3_resource

    return mock_s3_bucket


def get_call_args(call_args_list):
    arg_list = []
    for call in call_args_list:
        args, _ = call
        arg_list.append(args)
    return arg_list


def expected_call_args_list_single_obj(dest, path):
    return [(f"{path}".strip("/"), f'{dest}/{path.rsplit("/", 1)[-1]}'.strip("/"))]


def expected_call_args_list(parent_key, dest, paths):
    return [(f"{parent_key}/{p}".strip("/"), f"{dest}/{p}".strip("/")) for p in paths]


# pylint: disable=protected-access


@mock.patch("boto3.resource")
def test_parent_key(mock_storage):
    # given
    bucket_name = "foo"
    paths = ["models/weights.pt", "0002.h5", "a/very/long/path/config.json"]
    object_paths = ["bar/" + p for p in paths]

    # when
    mock_boto3_bucket = create_mock_boto3_bucket(mock_storage, object_paths)
    Storage._download_s3(f"s3://{bucket_name}/bar", "dest_path")

    # then
    arg_list = get_call_args(mock_boto3_bucket.download_file.call_args_list)
    assert arg_list == expected_call_args_list("bar", "dest_path", paths)

    mock_boto3_bucket.objects.filter.assert_called_with(Prefix="bar")


@mock.patch("boto3.resource")
def test_no_key(mock_storage):
    # given
    bucket_name = "foo"
    object_paths = ["models/weights.pt", "0002.h5", "a/very/long/path/config.json"]

    # when
    mock_boto3_bucket = create_mock_boto3_bucket(mock_storage, object_paths)
    Storage._download_s3(f"s3://{bucket_name}/", "dest_path")

    # then
    arg_list = get_call_args(mock_boto3_bucket.download_file.call_args_list)
    assert arg_list == expected_call_args_list("", "dest_path", object_paths)

    mock_boto3_bucket.objects.filter.assert_called_with(Prefix="")


@mock.patch("boto3.resource")
def test_storage_s3_exception(mock_resource):
    path = "s3://foo/bar"
    # Create mock client
    mock_s3_resource = mock.MagicMock()
    mock_s3_resource.Bucket.side_effect = Exception()
    mock_resource.return_value = mock_s3_resource

    with pytest.raises(Exception):
        Storage.download(path)


@mock.patch("boto3.resource")
@mock.patch("urllib3.PoolManager")
def test_no_permission_buckets(mock_connection, mock_resource):
    bad_s3_path = "s3://random/path"
    # Access private buckets without credentials
    mock_s3_resource = mock.MagicMock()
    mock_s3_bucket = mock.MagicMock()
    mock_s3_bucket.objects.filter.return_value = [mock.MagicMock()]
    mock_s3_bucket.objects.filter.side_effect = botocore.exceptions.ClientError(
        {}, "GetObject"
    )
    mock_s3_resource.Bucket.return_value = mock_s3_bucket
    mock_resource.return_value = mock_s3_resource

    with pytest.raises(botocore.exceptions.ClientError):
        Storage.download(bad_s3_path)


@mock.patch("boto3.resource")
def test_full_name_key(mock_storage):
    # given
    bucket_name = "foo"
    object_key = "path/to/model/name.pt"

    # when
    mock_boto3_bucket = create_mock_boto3_bucket(mock_storage, [object_key])
    Storage._download_s3(f"s3://{bucket_name}/{object_key}", "dest_path")

    # then
    arg_list = get_call_args(mock_boto3_bucket.download_file.call_args_list)
    assert arg_list == expected_call_args_list_single_obj("dest_path", object_key)

    mock_boto3_bucket.objects.filter.assert_called_with(Prefix=object_key)


@mock.patch("boto3.resource")
def test_full_name_key_root_bucket_dir(mock_resource):
    # given
    bucket_name = "foo"
    object_key = "name.pt"

    # when
    mock_boto3_bucket = create_mock_boto3_bucket(mock_resource, [object_key])
    Storage._download_s3(f"s3://{bucket_name}/{object_key}", "dest_path")

    # then
    arg_list = get_call_args(mock_boto3_bucket.download_file.call_args_list)
    assert arg_list == expected_call_args_list_single_obj("dest_path", object_key)

    mock_boto3_bucket.objects.filter.assert_called_with(Prefix=object_key)


AWS_TEST_CREDENTIALS = {
    "AWS_ACCESS_KEY_ID": "testing",
    "AWS_SECRET_ACCESS_KEY": "testing",
    "AWS_SECURITY_TOKEN": "testing",
    "AWS_SESSION_TOKEN": "testing",
}


@mock.patch("boto3.resource")
def test_multikey(mock_storage):
    # given
    bucket_name = "foo"
    paths = ["b/model.bin"]
    object_paths = ["test/a/" + p for p in paths]

    # when
    mock_boto3_bucket = create_mock_boto3_bucket(mock_storage, object_paths)
    Storage._download_s3(f"s3://{bucket_name}/test/a", "dest_path")

    # then
    arg_list = get_call_args(mock_boto3_bucket.download_file.call_args_list)
    assert arg_list == expected_call_args_list("test/a", "dest_path", paths)

    mock_boto3_bucket.objects.filter.assert_called_with(Prefix="test/a")


@mock.patch("boto3.resource")
def test_files_with_no_extension(mock_storage):

    # given
    bucket_name = "foo"
    paths = ["churn-pickle", "churn-pickle-logs", "churn-pickle-report"]
    object_paths = ["test/" + p for p in paths]

    # when
    mock_boto3_bucket = create_mock_boto3_bucket(mock_storage, object_paths)
    Storage._download_s3(f"s3://{bucket_name}/test/churn-pickle", "dest_path")

    # then
    arg_list = get_call_args(mock_boto3_bucket.download_file.call_args_list)

    # Download only the exact file if found; otherwise, download all files with the given prefix
    assert arg_list[0] == expected_call_args_list("test", "dest_path", paths)[0]

    mock_boto3_bucket.objects.filter.assert_called_with(Prefix="test/churn-pickle")


def test_get_S3_config():
    DEFAULT_CONFIG = Config()
    ANON_CONFIG = Config(signature_version=UNSIGNED)
    VIRTUAL_CONFIG = Config(s3={"addressing_style": "virtual"})
    USE_ACCELERATE_CONFIG = Config(s3={"use_accelerate_endpoint": True})

    with mock.patch.dict(os.environ, {}):
        config1 = Storage.get_S3_config()
    assert vars(config1) == vars(DEFAULT_CONFIG)

    with mock.patch.dict(os.environ, {"awsAnonymousCredential": "False"}):
        config2 = Storage.get_S3_config()
    assert vars(config2) == vars(DEFAULT_CONFIG)

    with mock.patch.dict(os.environ, AWS_TEST_CREDENTIALS):
        config3 = Storage.get_S3_config()
    assert vars(config3) == vars(DEFAULT_CONFIG)

    with mock.patch.dict(os.environ, {"awsAnonymousCredential": "True"}):
        config4 = Storage.get_S3_config()
    assert config4.signature_version == ANON_CONFIG.signature_version

    # assuming Python 3.5 or greater for joining dictionaries
    credentials_and_anon = {**AWS_TEST_CREDENTIALS, "awsAnonymousCredential": "True"}
    with mock.patch.dict(os.environ, credentials_and_anon):
        config5 = Storage.get_S3_config()
    assert config5.signature_version == ANON_CONFIG.signature_version

    with mock.patch.dict(os.environ, {"S3_USER_VIRTUAL_BUCKET": "False"}):
        config6 = Storage.get_S3_config()
    assert vars(config6) == vars(DEFAULT_CONFIG)

    with mock.patch.dict(os.environ, {"S3_USER_VIRTUAL_BUCKET": "True"}):
        config7 = Storage.get_S3_config()
    assert config7.s3["addressing_style"] == VIRTUAL_CONFIG.s3["addressing_style"]

    with mock.patch.dict(os.environ, {"S3_USE_ACCELERATE": "False"}):
        config6 = Storage.get_S3_config()
    assert vars(config6) == vars(DEFAULT_CONFIG)

    with mock.patch.dict(os.environ, {"S3_USE_ACCELERATE": "True"}):
        config7 = Storage.get_S3_config()
    assert (
        config7.s3["use_accelerate_endpoint"]
        == USE_ACCELERATE_CONFIG.s3["use_accelerate_endpoint"]
    )

    # tests legacy endpoint url
    with mock.patch.dict(
        os.environ,
        {
            "AWS_ENDPOINT_URL": "https://s3.amazonaws.com",
            "AWS_DEFAULT_REGION": "eu-west-1",
        },
    ):
        config8 = Storage.get_S3_config()
    assert config8.s3["addressing_style"] == VIRTUAL_CONFIG.s3["addressing_style"]


def test_update_with_storage_spec_s3(monkeypatch):
    # save the environment and restore it after the test to avoid mutating it
    # since _update_with_storage_spec modifies it
    previous_env = os.environ.copy()

    monkeypatch.setenv("STORAGE_CONFIG", '{"type": "s3"}')
    Storage._update_with_storage_spec()

    for var in (
        "AWS_ENDPOINT_URL",
        "AWS_ACCESS_KEY_ID",
        "AWS_SECRET_ACCESS_KEY",
        "AWS_DEFAULT_REGION",
        "AWS_CA_BUNDLE",
        "S3_VERIFY_SSL",
        "awsAnonymousCredential",
    ):
        assert os.getenv(var) is None

    storage_config = {
        "access_key_id": "xxxxxxxxxxxxxxxxxxxx",
        "bucket": "abucketname",
        "default_bucket": "abucketname",
        "endpoint_url": "https://s3.us-east-2.amazonaws.com/",
        "region": "us-east-2",
        "secret_access_key": "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
        "type": "s3",
        "ca_bundle": "/path/to/cabundle.crt",
        "verify_ssl": "false",
        "anonymous": "True",
    }

    monkeypatch.setenv("STORAGE_CONFIG", json.dumps(storage_config))
    Storage._update_with_storage_spec()

    assert os.getenv("AWS_ENDPOINT_URL") == storage_config["endpoint_url"]
    assert os.getenv("AWS_ACCESS_KEY_ID") == storage_config["access_key_id"]
    assert os.getenv("AWS_SECRET_ACCESS_KEY") == storage_config["secret_access_key"]
    assert os.getenv("AWS_DEFAULT_REGION") == storage_config["region"]
    assert os.getenv("AWS_CA_BUNDLE") == storage_config["ca_bundle"]
    assert os.getenv("S3_VERIFY_SSL") == storage_config["verify_ssl"]
    assert os.getenv("awsAnonymousCredential") == storage_config["anonymous"]

    # revert changes
    os.environ.clear()
    os.environ.update(previous_env)


@mock.patch("boto3.resource")
def test_target_startswith_parent_folder_name(mock_storage):
    bucket_name = "foo"
    paths = ["model.pkl", "a/model.pkl", "conda.yaml"]
    object_paths = ["test/artifacts/model/" + p for p in paths]

    # when
    mock_boto3_bucket = create_mock_boto3_bucket(mock_storage, object_paths)
    Storage._download_s3(f"s3://{bucket_name}/test/artifacts/model", "dest_path")

    # then
    arg_list = get_call_args(mock_boto3_bucket.download_file.call_args_list)
    assert (
        arg_list[0]
        == expected_call_args_list("test/artifacts/model", "dest_path", paths)[0]
    )
    mock_boto3_bucket.objects.filter.assert_called_with(Prefix="test/artifacts/model")


@mock.patch("boto3.resource")
def test_file_name_preservation(mock_storage):
    # given
    bucket_name = "local-model"
    paths = ["MLmodel"]
    object_paths = ["model/" + p for p in paths]
    expected_file_name = "MLmodel"  # Expected file name after download

    # when
    mock_boto3_bucket = create_mock_boto3_bucket(mock_storage, object_paths)
    Storage._download_s3(f"s3://{bucket_name}/model", "dest_path")

    # then
    arg_list = get_call_args(mock_boto3_bucket.download_file.call_args_list)
    assert len(arg_list) == 1  # Ensure only one file was downloaded
    downloaded_source, downloaded_target = arg_list[0]

    # Check if the source S3 key matches the original object key
    assert (
        downloaded_source == object_paths[0]
    ), f"Expected {object_paths[0]}, got {downloaded_source}"

    # Check if the target file path ends with the expected file name
    assert downloaded_target.endswith(
        expected_file_name
    ), f"Expected file name to end with {expected_file_name}, got {downloaded_target}"

    mock_boto3_bucket.objects.filter.assert_called_with(Prefix="model")


@mock.patch("boto3.resource")
def test_target_download_path_and_name(mock_storage):
    bucket_name = "foo"
    paths = ["model.pkl", "a/model.pkl", "conda.yaml"]
    object_paths = ["model/" + p for p in paths]

    # when
    mock_boto3_bucket = create_mock_boto3_bucket(mock_storage, object_paths)
    Storage._download_s3(f"s3://{bucket_name}/model", "dest_path")

    # then
    arg_list = get_call_args(mock_boto3_bucket.download_file.call_args_list)
    assert arg_list[0] == expected_call_args_list("model", "dest_path", paths)[0]
    assert arg_list[1] == expected_call_args_list("model", "dest_path", paths)[1]

    mock_boto3_bucket.objects.filter.assert_called_with(Prefix="model")


@mock.patch("boto3.resource")
def test_ca_bundle_with_aws_ca_bundle_only(mock_storage):
    """Test that AWS_CA_BUNDLE can be used independently without CA_BUNDLE_CONFIGMAP_NAME"""
    bucket_name = "foo"
    object_key = "model.pkl"

    # Create a temporary CA bundle file
    with tempfile.NamedTemporaryFile(
        mode="w", delete=False, suffix=".crt"
    ) as temp_ca_file:
        temp_ca_file.write(
            "-----BEGIN CERTIFICATE-----\ntest\n-----END CERTIFICATE-----"
        )
        ca_bundle_path = temp_ca_file.name

    try:
        # Mock the boto3 resource and bucket
        create_mock_boto3_bucket(mock_storage, [object_key])

        # Set only AWS_CA_BUNDLE environment variable (no CA_BUNDLE_CONFIGMAP_NAME)
        with mock.patch.dict(
            os.environ,
            {"AWS_CA_BUNDLE": ca_bundle_path, "S3_VERIFY_SSL": "true"},
            clear=True,
        ):
            Storage._download_s3(f"s3://{bucket_name}/{object_key}", "dest_path")

        # Verify that boto3.resource was called twice (once for listing, once for download)
        # Both calls should have the correct verify parameter
        assert mock_storage.call_count == 2
        for call_args in mock_storage.call_args_list:
            assert call_args[1]["verify"] == ca_bundle_path

    finally:
        # Clean up the temporary file
        os.unlink(ca_bundle_path)


@mock.patch("boto3.resource")
def test_ca_bundle_with_configmap_only(mock_storage):
    """Test that CA bundle works with ConfigMap when AWS_CA_BUNDLE is not set"""
    bucket_name = "foo"
    object_key = "model.pkl"

    # Create a temporary CA bundle file in the expected ConfigMap location
    with tempfile.TemporaryDirectory() as temp_dir:
        ca_bundle_path = os.path.join(temp_dir, "cabundle.crt")
        with open(ca_bundle_path, "w") as f:
            f.write("-----BEGIN CERTIFICATE-----\ntest\n-----END CERTIFICATE-----")

        # Mock the boto3 resource and bucket
        create_mock_boto3_bucket(mock_storage, [object_key])

        # Set ConfigMap environment variables only
        with mock.patch.dict(
            os.environ,
            {
                "CA_BUNDLE_CONFIGMAP_NAME": "test-configmap",
                "CA_BUNDLE_VOLUME_MOUNT_POINT": temp_dir,
                "S3_VERIFY_SSL": "true",
            },
            clear=True,
        ):
            Storage._download_s3(f"s3://{bucket_name}/{object_key}", "dest_path")

        # Verify that boto3.resource was called twice (once for listing, once for download)
        # Both calls should have the correct verify parameter
        assert mock_storage.call_count == 2
        for call_args in mock_storage.call_args_list:
            assert call_args[1]["verify"] == ca_bundle_path


@mock.patch("boto3.resource")
def test_ca_bundle_aws_ca_bundle_takes_precedence(mock_storage):
    """Test that AWS_CA_BUNDLE takes precedence over ConfigMap when both are set"""
    bucket_name = "foo"
    object_key = "model.pkl"

    # Create temporary CA bundle files
    with tempfile.NamedTemporaryFile(
        mode="w", delete=False, suffix=".crt"
    ) as aws_ca_file:
        aws_ca_file.write(
            "-----BEGIN CERTIFICATE-----\naws_ca\n-----END CERTIFICATE-----"
        )
        aws_ca_bundle_path = aws_ca_file.name

    with tempfile.TemporaryDirectory() as temp_dir:
        configmap_ca_bundle_path = os.path.join(temp_dir, "cabundle.crt")
        with open(configmap_ca_bundle_path, "w") as f:
            f.write(
                "-----BEGIN CERTIFICATE-----\nconfigmap_ca\n-----END CERTIFICATE-----"
            )

        try:
            # Mock the boto3 resource and bucket
            create_mock_boto3_bucket(mock_storage, [object_key])

            # Set both AWS_CA_BUNDLE and ConfigMap environment variables
            with mock.patch.dict(
                os.environ,
                {
                    "AWS_CA_BUNDLE": aws_ca_bundle_path,
                    "CA_BUNDLE_CONFIGMAP_NAME": "test-configmap",
                    "CA_BUNDLE_VOLUME_MOUNT_POINT": temp_dir,
                    "S3_VERIFY_SSL": "true",
                },
                clear=True,
            ):
                Storage._download_s3(f"s3://{bucket_name}/{object_key}", "dest_path")

            # Verify that boto3.resource was called twice with AWS_CA_BUNDLE path (takes precedence)
            assert mock_storage.call_count == 2
            for call_args in mock_storage.call_args_list:
                assert call_args[1]["verify"] == aws_ca_bundle_path

        finally:
            # Clean up the temporary file
            os.unlink(aws_ca_bundle_path)


@mock.patch("boto3.resource")
def test_ca_bundle_file_not_found_aws_ca_bundle(mock_storage):
    """Test that RuntimeError is raised when AWS_CA_BUNDLE file doesn't exist"""
    bucket_name = "foo"
    object_key = "model.pkl"
    non_existent_path = "/tmp/non_existent_ca_bundle.crt"

    # Mock the boto3 resource and bucket
    create_mock_boto3_bucket(mock_storage, [object_key])

    # Set AWS_CA_BUNDLE to a non-existent file
    with mock.patch.dict(
        os.environ,
        {"AWS_CA_BUNDLE": non_existent_path, "S3_VERIFY_SSL": "true"},
        clear=True,
    ):
        with pytest.raises(
            RuntimeError,
            match=f"Failed to find ca bundle file\\({non_existent_path}\\)",
        ):
            Storage._download_s3(f"s3://{bucket_name}/{object_key}", "dest_path")


@mock.patch("boto3.resource")
def test_ca_bundle_file_not_found_configmap(mock_storage):
    """Test that RuntimeError is raised when ConfigMap CA bundle file doesn't exist"""
    bucket_name = "foo"
    object_key = "model.pkl"

    # Mock the boto3 resource and bucket
    create_mock_boto3_bucket(mock_storage, [object_key])

    # Set ConfigMap environment variables with non-existent directory
    with mock.patch.dict(
        os.environ,
        {
            "CA_BUNDLE_CONFIGMAP_NAME": "test-configmap",
            "CA_BUNDLE_VOLUME_MOUNT_POINT": "/tmp/non_existent_dir",
            "S3_VERIFY_SSL": "true",
        },
        clear=True,
    ):
        expected_path = "/tmp/non_existent_dir/cabundle.crt"
        with pytest.raises(
            RuntimeError, match=f"Failed to find ca bundle file\\({expected_path}\\)"
        ):
            Storage._download_s3(f"s3://{bucket_name}/{object_key}", "dest_path")


@mock.patch("boto3.resource")
def test_ca_bundle_verify_ssl_false_no_ca_bundle_check(mock_storage):
    """Test that CA bundle is not checked when S3_VERIFY_SSL is false"""
    bucket_name = "foo"
    object_key = "model.pkl"
    non_existent_path = "/tmp/non_existent_ca_bundle.crt"

    # Mock the boto3 resource and bucket
    create_mock_boto3_bucket(mock_storage, [object_key])

    # Set AWS_CA_BUNDLE to a non-existent file but disable SSL verification
    with mock.patch.dict(
        os.environ,
        {"AWS_CA_BUNDLE": non_existent_path, "S3_VERIFY_SSL": "false"},
        clear=True,
    ):
        # This should not raise an error because verify_ssl is False
        Storage._download_s3(f"s3://{bucket_name}/{object_key}", "dest_path")

    # Verify that boto3.resource was called twice with verify=False
    assert mock_storage.call_count == 2
    for call_args in mock_storage.call_args_list:
        assert call_args[1]["verify"] is False


@mock.patch("boto3.resource")
def test_ca_bundle_empty_aws_ca_bundle_uses_configmap(mock_storage):
    """Test that empty AWS_CA_BUNDLE falls back to ConfigMap"""
    bucket_name = "foo"
    object_key = "model.pkl"

    # Create a temporary CA bundle file in the expected ConfigMap location
    with tempfile.TemporaryDirectory() as temp_dir:
        ca_bundle_path = os.path.join(temp_dir, "cabundle.crt")
        with open(ca_bundle_path, "w") as f:
            f.write("-----BEGIN CERTIFICATE-----\ntest\n-----END CERTIFICATE-----")

        # Mock the boto3 resource and bucket
        create_mock_boto3_bucket(mock_storage, [object_key])

        # Set empty AWS_CA_BUNDLE and ConfigMap environment variables
        with mock.patch.dict(
            os.environ,
            {
                "AWS_CA_BUNDLE": "",
                "CA_BUNDLE_CONFIGMAP_NAME": "test-configmap",
                "CA_BUNDLE_VOLUME_MOUNT_POINT": temp_dir,
                "S3_VERIFY_SSL": "true",
            },
            clear=True,
        ):
            Storage._download_s3(f"s3://{bucket_name}/{object_key}", "dest_path")

        # Verify that boto3.resource was called twice with the ConfigMap path
        assert mock_storage.call_count == 2
        for call_args in mock_storage.call_args_list:
            assert call_args[1]["verify"] == ca_bundle_path
