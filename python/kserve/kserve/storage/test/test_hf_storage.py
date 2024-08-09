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


def test_download_hf():
    uri = "hf://example.com/model:hash_value"

    mock_tokenizer_instance = mock.MagicMock()
    patch_tokenizer = mock.patch(
        "transformers.AutoTokenizer.from_pretrained",
        return_value=mock_tokenizer_instance,
    )

    mock_config_instance = mock.MagicMock()
    patch_config = mock.patch(
        "transformers.AutoConfig.from_pretrained", return_value=mock_config_instance
    )

    mock_model_instance = mock.MagicMock()
    patch_model = mock.patch(
        "transformers.AutoModel.from_config", return_value=mock_model_instance
    )

    with patch_tokenizer, patch_config, patch_model:
        Storage.download(uri)

    mock_tokenizer_instance.save_pretrained.assert_called_once()
    mock_model_instance.save_pretrained.assert_called_once()
