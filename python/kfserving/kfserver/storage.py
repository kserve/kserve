import os
import shutil
import errno
import logging
import random
_GCS_PREFIX = "gs://"
_S3_PREFIX = "s3://"
_LOCAL_PREFIX = "/"

import tempfile


class Storage(object):
    @staticmethod
    def download(uri):
        if uri.startswith(_LOCAL_PREFIX):
            return uri

        temp_dir = tempfile.mkdtemp()
        if uri.startswith(_GCS_PREFIX):
            Storage._download_gcs(uri, temp_dir)
        elif uri.startswith(_S3_PREFIX):
            Storage._download_s3(uri, temp_dir)
        else:
            raise Exception("Cannot recognize storage type for " + uri)
        return temp_dir

    @staticmethod
    def _download_s3(uri, temp_dir):
        raise NotImplementedError

    @staticmethod
    def _download_gcs(uri, temp_dir):
        raise NotImplementedError
