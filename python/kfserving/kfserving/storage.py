# Copyright 2019 kubeflow.org.
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

import logging
import tempfile
import os

_GCS_PREFIX = "gs://"
_S3_PREFIX = "s3://"
_LOCAL_PREFIX = "file://"


class Storage(object):
    @staticmethod
    def download(uri: str) -> str:
        logging.info("Copying contents of %s to local" % uri)
        if uri.startswith(_LOCAL_PREFIX) or os.path.exists(uri):
            return Storage._download_local(uri)

        temp_dir = tempfile.mkdtemp()
        if uri.startswith(_GCS_PREFIX):
            Storage._download_gcs(uri, temp_dir)
        elif uri.startswith(_S3_PREFIX):
            Storage._download_s3(uri, temp_dir)
        else:
            raise Exception("Cannot recognize storage type for " + uri +
                            "\n'%s', '%s', and '%s' are the current available storage type." %
                            (_GCS_PREFIX, _S3_PREFIX, _LOCAL_PREFIX))

        logging.info("Successfully copied %s to %s" % (uri, temp_dir))
        return temp_dir

    @staticmethod
    def _download_s3(uri, temp_dir: str):
        raise NotImplementedError

    @staticmethod
    def _download_gcs(uri, temp_dir: str):
        raise NotImplementedError

    @staticmethod
    def _download_local(uri):
        local_path = uri.replace(_LOCAL_PREFIX, "", 1)
        if not os.path.exists(local_path):
            raise Exception("Local path %s does not exist." % (uri))
        return local_path
