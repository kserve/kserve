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

from abc import ABC, abstractmethod


class SecretResolutionError(Exception):
    """Raised when a decryption key cannot be resolved."""


class SecretResolver(ABC):
    """Abstract interface for resolving decryption keys from a resource identifier.

    Implementations contact a key management service (e.g. KBS) to retrieve
    the symmetric key needed to decrypt JWE-encrypted model artifacts.
    """

    @abstractmethod
    def resolve_key(self, resource_id: str) -> bytes:
        """Resolve a decryption key for the given resource identifier.

        Args:
            resource_id: A KBS resource identifier, typically in the format
                ``kbs:///<repo>/<type>/<tag>``.

        Returns:
            The raw key bytes for decryption.

        Raises:
            SecretResolutionError: If the key cannot be retrieved.
        """
