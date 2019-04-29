import os
import shutil
import errno
import logging

_GCS_PREFIX = "gs://"
_S3_PREFIX = "s3://"
_LOCAL_PREFIX = "/"


class Storage(object):
    @staticmethod
    def download(uri, local_dir):
        if uri.startswith(_GCS_PREFIX):
            Storage._download_gcs(uri, local_dir)
        elif uri.startswith(_S3_PREFIX):
            Storage._download_s3(uri, local_dir)
        elif uri.startswith(_LOCAL_PREFIX):
            Storage._copy_local(uri, local_dir)
        else:
            raise Exception("Cannot recognize storage type for " + uri)

    @staticmethod
    def _download_s3(uri, local_dir):
        raise NotImplementedError

    @staticmethod
    def _download_gcs(uri, local_dir):
        raise NotImplementedError

    @staticmethod
    def _copy_local(uri, local_dir):
        try:
            logging.warn("making %s" % local_dir)
            # os.makedirs(local_dir, exist_ok=True)
            
            logging.warn("copying %s %s" % (uri, local_dir))
            shutil.copytree(uri, local_dir)
        except OSError as e:
            if e.errno == errno.ENOTDIR:
                logging.warn("Expected {} to be a directory, treating as a file instead")
                shutil.copy(uri, local_dir)
            else:
                raise Exception("Unable to copy URI %s" % e)
