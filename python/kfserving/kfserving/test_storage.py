import pytest
import kfserving

def test_storage_local_path():
    abs_path = '/tmp/file'
    relative_path = '.'
    assert kfserving.Storage.download(abs_path) == abs_path
    assert kfserving.Storage.download(relative_path) == relative_path
