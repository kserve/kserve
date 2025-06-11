# kserve-storage

A Python module for handling model storage and retrieval for KServe. This package provides a unified API to download models from various storage backends including cloud providers, file systems, and model hubs.

## Features

- Support for multiple storage backends:
    - Local file system
    - Google Cloud Storage (GCS)
    - Amazon S3
    - Azure Blob Storage
    - Azure File Share
    - HTTP/HTTPS URLs
    - HDFS/WebHDFS
    - Hugging Face Hub
- Automatic extraction of compressed files (zip, tar.gz, tgz)
- Configuration via environment variables
- Logging and error handling

## Installation

```bash
pip install kserve-storage
```

Or with Poetry:

```bash
poetry add kserve-storage
```

## Usage

The main entry point is the `Storage` class which provides a `download` method:

```python
from kserve_storage import Storage

# Download from GCS to a temporary directory
model_dir = Storage.download("gs://your-bucket/model")

# Download from S3 to a specific directory
model_dir = Storage.download("s3://your-bucket/model", "/path/to/destination")
```

## Supported Storage Providers

### Local File System

```python
model_dir = Storage.download("file:///path/to/model")
# or using direct path
model_dir = Storage.download("/path/to/model")
```

### Google Cloud Storage

```python
model_dir = Storage.download("gs://bucket-name/model-path")
```

### Amazon S3

```python
model_dir = Storage.download("s3://bucket-name/model-path")
```

### Azure Blob Storage

```python
model_dir = Storage.download("https://account-name.blob.core.windows.net/container-name/model-path")
```

### Azure File Share

```python
model_dir = Storage.download("https://account-name.file.core.windows.net/share-name/model-path")
```

### HTTP/HTTPS URLs

```python
model_dir = Storage.download("https://example.com/path/to/model.zip")
```

### HDFS

```python
model_dir = Storage.download("hdfs://path/to/model")
# or WebHDFS
model_dir = Storage.download("webhdfs://path/to/model")
```

### Hugging Face Hub

```python
model_dir = Storage.download("hf://org-name/model-name")
# With specific revision
model_dir = Storage.download("hf://org-name/model-name:revision")
```

## Environment Variables

### Hugging Face Hub Configuration

These are all handled by the `huggingface_hub` package, you can see all the available environment variables [here](https://huggingface.co/docs/huggingface_hub/en/package_reference/environment_variables).

### AWS/S3 Configuration / Environments variables

- `AWS_ENDPOINT_URL`: Custom endpoint URL for S3-compatible storage
- `AWS_ACCESS_KEY_ID`: Access key for S3
- `AWS_SECRET_ACCESS_KEY`: Secret access key for S3
- `AWS_DEFAULT_REGION`: AWS region
- `AWS_CA_BUNDLE`: Path to custom CA bundle
- `S3_VERIFY_SSL`: Enable/disable SSL verification
- `S3_USER_VIRTUAL_BUCKET`: Use virtual hosted-style URLs
- `S3_USE_ACCELERATE`: Use transfer acceleration
- `awsAnonymousCredential`: Use unsigned requests for public access

### Azure Configuration

- `AZURE_STORAGE_ACCESS_KEY`: Storage account access key
- `AZ_TENANT_ID` / `AZURE_TENANT_ID`: Azure AD tenant ID
- `AZ_CLIENT_ID` / `AZURE_CLIENT_ID`: Azure AD client ID
- `AZ_CLIENT_SECRET` / `AZURE_CLIENT_SECRET`: Azure AD client secret

### HDFS Configuration

- `HDFS_SECRET_DIR`: Directory containing HDFS configuration files
- `HDFS_NAMENODE`: HDFS namenode address
- `USER_PROXY`: User proxy for HDFS
- `HDFS_ROOTPATH`: Root path in HDFS
- `KERBEROS_PRINCIPAL`: Kerberos principal for authentication
- `KERBEROS_KEYTAB`: Path to Kerberos keytab file
- `TLS_CERT`, `TLS_KEY`, `TLS_CA`: TLS configuration files
- `TLS_SKIP_VERIFY`: Skip TLS verification
- `N_THREADS`: Number of download threads

## Storage Configuration

Storage configuration can be provided through environment variables:

- `STORAGE_CONFIG`: JSON string containing storage configuration
- `STORAGE_OVERRIDE_CONFIG`: JSON string to override storage configuration


## License

Apache License 2.0 - See [LICENSE](https://github.com/kserve/kserve/blob/master/LICENSE) for details.