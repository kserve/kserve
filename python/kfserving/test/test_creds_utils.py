# Copyright 2020 kubeflow.org.
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

from unittest import mock

from kubernetes.client import V1ServiceAccountList, V1ServiceAccount, V1ObjectMeta

from kfserving.api.creds_utils import check_sa_exists


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
