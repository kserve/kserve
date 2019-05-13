import logging
import tempfile
import os

_GCS_PREFIX = "gs://"
_S3_PREFIX = "s3://"
_LOCAL_PREFIX = "/"


class Storage(object):
    @staticmethod
    def download(uri: str) -> str:
        logging.info("Copying contents of %s to local" % uri)
        if uri.startswith(_LOCAL_PREFIX) or os.path.exists(uri):
            return uri

        temp_dir = tempfile.mkdtemp()
        if uri.startswith(_GCS_PREFIX):
            Storage._download_gcs(uri, temp_dir)
        elif uri.startswith(_S3_PREFIX):
            Storage._download_s3(uri, temp_dir)
        else:
            raise Exception("Cannot recognize storage type for " + uri)

        logging.info("Successfully copied %s to %s" % (uri, temp_dir))
        return temp_dir

    @staticmethod
    def _download_s3(uri, temp_dir: str):
        raise NotImplementedError

    @staticmethod
    def _download_gcs(uri, temp_dir: str):
        raise NotImplementedError
