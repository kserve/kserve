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

import json
import tempfile
from unittest import mock
import os
import re

import pytest
from kubernetes.client import (V1ObjectMeta, V1ServiceAccount,
                               V1ServiceAccountList, rest)

from kserve import constants
from kserve.api.creds_utils import (check_sa_exists, create_secret,
                                    create_service_account,
                                    get_creds_name_from_config_map,
                                    patch_service_account,
                                    set_azure_credentials, set_gcs_credentials,
                                    set_s3_credentials, set_service_account,
                                    parse_grpc_server_credentials)


@mock.patch('kubernetes.client.CoreV1Api.list_namespaced_service_account')
def test_check_sa_exists(mock_client):
    # Mock kubernetes client to return 2 accounts
    accounts = V1ServiceAccountList(
        items=[V1ServiceAccount(metadata=V1ObjectMeta(name=n)) for n in ['a', 'b']]
    )
    mock_client.return_value = accounts

    # then a, b should exist, c should not
    assert check_sa_exists('kubeflow', 'a') is True
    assert check_sa_exists('kubeflow', 'b') is True
    assert check_sa_exists('kubeflow', 'c') is False


@mock.patch('kubernetes.client.CoreV1Api.create_namespaced_service_account')
def test_create_service_account(mock_client):
    sa_name = "test"
    namespace = "kserve-test"
    secret_name = "test_secret"
    create_service_account(secret_name, namespace, sa_name)
    mock_client.assert_called_once()

    mock_client.side_effect = rest.ApiException('foo')
    with pytest.raises(RuntimeError):
        sa_name = "test"
        namespace = "kserve-test"
        secret_name = "test_secret"
        create_service_account(secret_name, namespace, sa_name)


@mock.patch('kubernetes.client.CoreV1Api.patch_namespaced_service_account')
def test_patch_service_account(mock_client):
    sa_name = "test"
    namespace = "kserve-test"
    secret_name = "test_secret"
    patch_service_account(secret_name, namespace, sa_name)
    mock_client.assert_called_once()

    mock_client.side_effect = rest.ApiException('foo')
    with pytest.raises(RuntimeError):
        sa_name = "test"
        namespace = "kserve-test"
        secret_name = "test_secret"
        patch_service_account(secret_name, namespace, sa_name)


@mock.patch('kubernetes.client.CoreV1Api.create_namespaced_secret')
def test_create_secret(mock_create_secret):
    namespace = "test"
    secret_name = "test-secret"
    mock_create_secret.return_value = mock.Mock(**{"metadata.name": secret_name})
    assert create_secret(namespace) == secret_name

    with pytest.raises(RuntimeError):
        mock_create_secret.side_effect = rest.ApiException('foo')
        create_secret(namespace)


@mock.patch('kserve.api.creds_utils.create_service_account')
@mock.patch('kserve.api.creds_utils.patch_service_account')
@mock.patch('kserve.api.creds_utils.check_sa_exists')
def test_set_service_account(mock_check_sa_exists, mock_patch_service_account, mock_create_service_account):
    namespace = "test"
    service_account = V1ServiceAccount()
    secret_name = "test-secret"
    mock_check_sa_exists.return_value = True
    set_service_account(namespace, service_account, secret_name)
    mock_patch_service_account.assert_called_once()

    mock_check_sa_exists.return_value = False
    set_service_account(namespace, service_account, secret_name)
    mock_create_service_account.assert_called_once()


@mock.patch('kubernetes.client.CoreV1Api.read_namespaced_config_map')
def test_get_creds_name_from_config_map(mock_read_config_map):
    mock_read_config_map.return_value = mock.Mock(**{"data": {"credentials": """{
        "gcs": {"gcsCredentialFileName": "gcs_cred.json"},
        "s3": {"s3AccessKeyIDName": "s3_access_key.json",
               "s3SecretAccessKeyName": "s3_secret.json"}}"""
                                                              }})
    test_cases = {'gcsCredentialFileName': 'gcs_cred.json',
                  's3AccessKeyIDName': 's3_access_key.json',
                  's3SecretAccessKeyName': 's3_secret.json'}
    for cred, result in test_cases.items():
        assert get_creds_name_from_config_map(cred) == result

    with pytest.raises(RuntimeError):
        get_creds_name_from_config_map("invalidCred")

    mock_read_config_map.side_effect = rest.ApiException('foo')
    assert get_creds_name_from_config_map('gcsCredentialFileName') is None


@mock.patch('kserve.api.creds_utils.set_service_account')
@mock.patch('kserve.api.creds_utils.create_secret')
@mock.patch('kserve.api.creds_utils.get_creds_name_from_config_map')
def test_set_gcs_credentials(mock_get_creds_name, mock_create_secret, mock_set_service_account):
    namespace = "test"
    service_account = V1ServiceAccount()
    temp_cred_file = tempfile.NamedTemporaryFile(suffix=".json")
    cred_file_name = temp_cred_file.name
    mock_get_creds_name.return_value = cred_file_name
    mock_create_secret.return_value = "test-secret"
    set_gcs_credentials(namespace, cred_file_name, service_account)
    mock_get_creds_name.assert_called()
    mock_create_secret.assert_called()
    mock_set_service_account.assert_called()

    mock_get_creds_name.return_value = None
    set_gcs_credentials(namespace, cred_file_name, service_account)
    mock_get_creds_name.assert_called()
    mock_create_secret.assert_called()
    mock_set_service_account.assert_called()


@mock.patch('kserve.api.creds_utils.set_service_account')
@mock.patch('kserve.api.creds_utils.create_secret')
@mock.patch('kserve.api.creds_utils.get_creds_name_from_config_map')
def test_set_s3_credentials(mock_get_creds_name, mock_create_secret, mock_set_service_account):
    namespace = "test"
    endpoint = "https://s3.aws.com"
    region = "ap-south-1"
    use_https = True
    verfify_ssl = True
    cabundle = "/user/test/cert.pem"
    data = {
        constants.S3_ACCESS_KEY_ID_DEFAULT_NAME: "XXXXXXXXXXXX",
        constants.S3_SECRET_ACCESS_KEY_DEFAULT_NAME: "XXXXXXXXXXXX",
    }
    annotations = {constants.KSERVE_GROUP + "/s3-endpoint": endpoint,
                   constants.KSERVE_GROUP + "/s3-region": region,
                   constants.KSERVE_GROUP + "/s3-usehttps": use_https,
                   constants.KSERVE_GROUP + "/s3-verifyssl": verfify_ssl,
                   constants.KSERVE_GROUP + "/s3-cabundle": cabundle
                   }
    creds_str = b"""
    [default]
    aws_access_key_id = XXXXXXXXXXXX
    aws_secret_access_key = XXXXXXXXXXXX
    """

    with tempfile.NamedTemporaryFile() as creds_file:
        creds_file.write(creds_str)
        creds_file.seek(0)
        mock_get_creds_name.return_value = None
        mock_create_secret.return_value = "test-secret"
        set_s3_credentials(namespace, creds_file.name, V1ServiceAccount(), s3_endpoint=endpoint,
                           s3_region=region, s3_use_https=use_https, s3_verify_ssl=verfify_ssl,
                           s3_cabundle=cabundle)
    mock_create_secret.assert_called_with(namespace=namespace, annotations=annotations, data=data)
    mock_get_creds_name.asset_called()
    mock_set_service_account.assert_called()


@mock.patch('kserve.api.creds_utils.set_service_account')
@mock.patch('kserve.api.creds_utils.create_secret')
def test_set_azure_credentials(mock_create_secret, mock_set_service_account):
    namespace = "test"
    creds = {
        "clientId": "XXXXXXXXXXX",
        "clientSecret": "XXXXXXXXXXX",
        "subscriptionId": "XXXXXXXXXXX",
        "tenantId": "XXXXXXXXXXX"
    }
    data = {
        'AZURE_CLIENT_ID': creds['clientId'],
        'AZURE_CLIENT_SECRET': creds['clientSecret'],
        'AZURE_SUBSCRIPTION_ID': creds['subscriptionId'],
        'AZURE_TENANT_ID': creds['tenantId'],
    }
    with tempfile.NamedTemporaryFile(suffix=".json") as creds_file:
        creds_file.write(json.dumps(creds).encode("utf-8"))
        creds_file.seek(0)
        mock_create_secret.return_value = "test-secret"
        set_azure_credentials(namespace, creds_file.name, V1ServiceAccount())
    mock_create_secret.assert_called_with(namespace=namespace, data=data)
    mock_set_service_account.assert_called()


def test_parse_grpc_server_credentials_valid_file_path():
    temp_file = tempfile.NamedTemporaryFile(delete=False)
    temp_file.write(b"dummy_cert_content")
    temp_file.close()

    result = parse_grpc_server_credentials(temp_file.name)
    assert result == b"dummy_cert_content"

    os.remove(temp_file.name)


def test_parse_grpc_server_credentials_invalid_file_path():
    with pytest.raises(RuntimeError, match="File not found."):
        parse_grpc_server_credentials("non_existent_file_path")


def test_parse_grpc_server_credentials_valid_bytes_input():
    result = parse_grpc_server_credentials(b"dummy_cert_content")
    assert result == b"dummy_cert_content"


def test_parse_grpc_server_credentials_invalid_type_input():
    expected_message = "SSL key must be of type string (file path to cert) or bytes (raw cert)."
    with pytest.raises(RuntimeError, match=re.escape(expected_message)):
        parse_grpc_server_credentials(12345)  # Invalid type
