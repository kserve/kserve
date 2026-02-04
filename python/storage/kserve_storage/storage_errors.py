# Copyright 2024 The KServe Authors.
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

"""
Storage error handling utilities.

This module provides unified error handling for all storage protocols,
converting low-level exceptions into user-friendly error messages.
"""

from .logging import logger


def raise_storage_error(
    protocol: str, uri: str, error: Exception, resource_name: str = ""
) -> None:
    """
    Unified error handler for all storage protocols.
    Logs the error and raises RuntimeError with a user-friendly message.

    Args:
        protocol: The storage protocol (e.g., "S3", "GCS", "Azure")
        uri: The storage URI being accessed
        error: The original exception
        resource_name: Optional resource name for context (e.g., bucket name)

    Raises:
        RuntimeError: With a user-friendly error message
    """
    error_msg = get_storage_error_message(protocol, error, resource_name)
    logger.error("%s error for %s: %s", protocol, uri, error)
    raise RuntimeError(error_msg) from error


def get_storage_error_message(
    protocol: str, error: Exception, resource_name: str = ""
) -> str:
    """
    Get user-friendly error message for storage errors across all protocols.

    Args:
        protocol: The storage protocol (e.g., "S3", "GCS", "Azure")
        error: The original exception
        resource_name: Optional resource name for context

    Returns:
        A user-friendly error message string
    """
    error_type = type(error).__name__
    error_str = str(error).lower()

    # Authentication errors
    auth_error_types = (
        "NoCredentialsError",
        "DefaultCredentialsError",
        "GoogleAuthError",
        "ClientAuthenticationError",
        "GatedRepoError",
    )
    if (
        error_type in auth_error_types
        or "credential" in error_str
        or "auth" in error_str
    ):
        auth_hints = {
            "S3": "Set AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY or use awsAnonymousCredential=true.",
            "GCS": "Verify GOOGLE_APPLICATION_CREDENTIALS or use anonymous access.",
            "Azure": "Verify AZURE_CLIENT_ID/AZURE_CLIENT_SECRET or AZURE_STORAGE_ACCESS_KEY.",
            "HDFS": "Verify Kerberos keytab and principal configuration.",
            "HuggingFace": "Set HF_TOKEN or request access to the gated repository.",
            "HTTP": "Verify authentication credentials.",
            "Git": "Set GIT_USERNAME/GIT_PASSWORD or use a public repository.",
        }
        return "%s authentication failed. %s" % (
            protocol,
            auth_hints.get(protocol, ""),
        )

    # Not found errors
    not_found_types = (
        "NotFound",
        "ResourceNotFoundError",
        "RepositoryNotFoundError",
        "RevisionNotFoundError",
    )
    if (
        error_type in not_found_types
        or "not found" in error_str
        or "does not exist" in error_str
    ):
        if resource_name:
            return "%s resource '%s' not found." % (protocol, resource_name)
        return "%s resource not found." % protocol

    # Access denied errors
    access_denied_types = ("Forbidden", "AccessDenied")
    if (
        error_type in access_denied_types
        or "access denied" in error_str
        or "permission" in error_str
    ):
        if resource_name:
            return "Access denied to %s resource '%s'. Verify permissions." % (
                protocol,
                resource_name,
            )
        return "Access denied. Verify %s permissions." % protocol

    # Connection errors
    if (
        error_type == "ConnectionError"
        or "connection" in error_str
        or "timeout" in error_str
    ):
        if resource_name:
            return "Failed to connect to %s endpoint '%s'." % (
                protocol,
                resource_name,
            )
        return "Failed to connect to %s endpoint." % protocol

    # S3-specific error codes
    if protocol == "S3" and hasattr(error, "response"):
        error_code = error.response.get("Error", {}).get("Code", "")
        if error_code in ("InvalidAccessKeyId", "SignatureDoesNotMatch"):
            return "Invalid AWS credentials. Verify AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY."
        if error_code == "NoSuchBucket":
            return "S3 bucket '%s' does not exist." % resource_name
        if error_code:
            return "S3 error [%s]: %s" % (
                error_code,
                error.response.get("Error", {}).get("Message", ""),
            )

    # Default: include error type and message
    return "%s error: %s" % (protocol, error)


def check_http_response(uri: str, response) -> None:
    """
    Check HTTP response status and raise RuntimeError for error codes.

    Args:
        uri: The HTTP URI being accessed
        response: The HTTP response object

    Raises:
        RuntimeError: If the response indicates an error
    """
    status_messages = {
        401: "HTTP authentication failed for '%s'. Verify credentials.",
        403: "HTTP access denied for '%s'. Verify permissions.",
        404: "HTTP resource not found at '%s'.",
    }
    if response.status_code in status_messages:
        raise RuntimeError(status_messages[response.status_code] % uri)
    if response.status_code != 200:
        raise RuntimeError(
            "HTTP request to '%s' failed with status code %s."
            % (uri, response.status_code)
        )
