import unittest.mock as mock
import pytest
from kserve.storage import Storage


STORAGE_MODULE = 'kserve.storage.storage'


def get_call_args(call_args_list):
    arg_list = []
    for call in call_args_list:
        args, _ = call
        arg_list.append(args)
    return arg_list

def create_mock_dir(name):
    mock_dir = mock.MagicMock()
    mock_dir.name = name
    return mock_dir

def create_mock_dir_with_file(dir_name, file_name):
    mock_obj = mock.MagicMock()
    mock_obj.name = f'{dir_name}/{file_name}'
    return mock_obj

def create_mock_bucket_list_blobs(mock_storage, list_blobs):
    mock_storage.Client().bucket().list_blobs().__iter__.return_value = list_blobs
    return mock_storage


@mock.patch(STORAGE_MODULE + '.storage')
def test_gcs_with_empty_dir(mock_storage):
    gcs_path = 'gs://foo/bar'

    create_mock_bucket_list_blobs(mock_storage, [create_mock_dir('bar/')])

    with pytest.raises(Exception):
        Storage.download(gcs_path)


@mock.patch(STORAGE_MODULE + '.storage')
def test_gcs_with_nested_sub_dir(mock_storage):
    gcs_path = 'gs://foo/bar/test'

    create_mock_bucket_list_blobs(
        mock_storage, [
            create_mock_dir('bar/'), 
            create_mock_dir('test/'), 
            create_mock_dir_with_file('test', 'mock.object')
            ]
        )
    Storage.download(gcs_path)

    arg_list = get_call_args(create_mock_dir_with_file('test', 'mock.object').download_to_filename.call_args_list)


def test_download_model_from_gcs():
    pass

def test_gcs_with_multiple_model():
    pass

def test_gcs_model_unpack_archive_file():
    pass
