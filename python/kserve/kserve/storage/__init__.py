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
# flake8: noqa

# Keep backwards compatibility, try import storage package.
# This way, existing projects using the Python SDK will not break, allowing
# users to upgrade the SDK without changing their code.
# We might need to inform users about the segregation of the storage package
# so they have time to update the code and change the imports to the new package.
try:
    from storage import Storage
except ImportError as e:
    raise ImportError("Failed to import storage package") from e

# Ensure that public methods are available in the kserve.storage namespace
download = Storage.download
get_S3_config = Storage.get_S3_config
