# Copyright 2023 The KServe Authors.
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

import unittest.mock as mock

from kserve.storage import Storage


@mock.patch("huggingface_hub.snapshot_download")
def test_download_model(mock_snapshot_download):
    uri = "hf://example.com/model:hash_value"
    repo = "example.com"
    model = "model"
    revision = "hash_value"

    Storage.download(uri)

    mock_snapshot_download.assert_called_once_with(
        repo_id=f"{repo}/{model}",
        revision=revision,
        local_dir=mock.ANY,
    )
