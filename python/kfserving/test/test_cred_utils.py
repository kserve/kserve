from unittest import mock

from kubernetes.client import V1ServiceAccountList, V1ServiceAccount, V1ObjectMeta

from kfserving.api.creds_utils import check_sa_exists


@mock.patch('kubernetes.client.CoreV1Api.list_namespaced_service_account')
def test_check_sa_exists(mock):
    # Mock kubernetes client to return 2 accounts
    accounts = V1ServiceAccountList(
        items=[V1ServiceAccount(metadata=V1ObjectMeta(name=n)) for n in ['a', 'b']]
    )
    mock.return_value = accounts

    # then a, b should exists, c should not exists
    assert check_sa_exists('kubeflow', 'a') is True
    assert check_sa_exists('kubeflow', 'b') is True
    assert check_sa_exists('kubeflow', 'c') is False
