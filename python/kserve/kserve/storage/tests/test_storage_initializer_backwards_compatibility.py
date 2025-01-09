# Copyright 2025 The KServe Authors.
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

import pytest


def test_kserve_module_not_installed():
    with pytest.raises(ImportError):
        import kserve.storage as Storage

        Storage.download("s3://test", "test")


def test_kserve_module_installed():
    import kserve.storage as Storage

    # it should return TypeError: Storage.download() missing 1 required positional argument: 'uri'
    with pytest.raises(TypeError):
        Storage.download()
