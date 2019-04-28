GCS_PREFIX="gs://"
S3_PREFIX="s3://"
LOCAL_PREFIX="/"

def uri_to_local(uri, local):
    if uri.startswith(GCS_PREFIX):
        return _gcs_to_local(uri, local)
    elif uri.startswith(S3_PREFIX):
        return _s3_to_local(uri, local)
    elif uri.startswith(LOCAL_PREFIX):
        return uri
    else:
        raise Exception("Cannot recognize storage type for " + uri)

def _s3_to_local(uri, local):
    raise NotImplementedError

def _gcs_to_local(uri, local):
    raise NotImplementedError