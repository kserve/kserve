# Python SDK templates

`api_client.mustache` is based on the Python template bundled with
OpenAPI Generator 4.3.1, the version pinned in `../client-gen.sh`.

It customizes model deserialization to:

- Look up Kubernetes models when a type is not defined by KServe. This includes
  nested types such as `V1SecretKeySelector`, which are not direct schema references.
- Accept both `dict(str, Type)` and `dict[str, Type]` annotations, so dictionary
  fields work with older and newer Kubernetes Python clients.

Keep these changes when upgrading the generator. Regenerate the SDK from the
repository root with `bash hack/python-sdk/client-gen.sh`, then run
`python -m pytest python/kserve/test/test_api_client_deserialization.py` in an
environment with the KServe package and its test dependencies installed.
