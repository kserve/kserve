# Credential Forwarding for Transformers

The transformer must forward the caller's credentials (the `Authorization` header) to the predictor so that the predictor's auth proxy (e.g. kube-rbac-proxy) can validate the original caller's identity.

## SDK-Based Transformers (Automatic)

If your transformer extends `kserve.Model` and uses the built-in `predict()` method (i.e., you override `preprocess` and/or `postprocess` but not `predict`), the KServe SDK **automatically forwards** the `Authorization` header from the incoming request to the predictor.

No additional configuration is needed. The following headers are forwarded automatically:

| Header | Purpose |
|--------|---------|
| `Authorization` | Caller credentials for predictor auth |
| `x-request-id` | Request tracing |
| `x-b3-traceid` | Distributed tracing (B3/Zipkin) |

### Example

```python
from kserve import Model, InferRequest, InferResponse

class MyTransformer(Model):
    def __init__(self, name: str):
        super().__init__(name)
        self.ready = True

    def preprocess(self, payload: InferRequest, headers=None) -> InferRequest:
        # Transform input - Authorization header is forwarded automatically
        # when the SDK calls predict() on the predictor
        return payload

    def postprocess(self, result: InferResponse, headers=None) -> InferResponse:
        return result
```

## Custom Transformers (Manual Forwarding Required)

If your transformer **overrides the `predict()` method** or makes its own HTTP calls to the predictor, you are responsible for forwarding the `Authorization` header yourself.

### What You Need To Do

1. Accept the `headers` parameter in your `predict` method
2. Extract the `Authorization` header from the incoming headers
3. Include it in the request you send to the predictor

### Example

```python
import httpx
from kserve import Model

class MyCustomTransformer(Model):
    def __init__(self, name: str):
        super().__init__(name)
        self.ready = True

    async def predict(self, payload, headers=None, response_headers=None):
        # Build headers for the predictor call
        predict_headers = {"Content-Type": "application/json"}

        # Forward the Authorization header for auth-enabled predictors
        if headers and "authorization" in headers:
            predict_headers["authorization"] = headers["authorization"]

        # Make the call to the predictor
        async with httpx.AsyncClient() as client:
            response = await client.post(
                f"http://{self.predictor_host}/v1/models/{self.name}:predict",
                json=payload,
                headers=predict_headers,
            )
            return response.json()
```

### Headers You Should Forward

At minimum, forward the `Authorization` header. For full compatibility with KServe observability, also forward:

- `x-request-id` - for request tracing
- `x-b3-traceid` - for distributed tracing

### Important Notes

- The **predictor authorizes the end user**, not the transformer service account. The transformer is a pass-through for credentials.
- The `Host` header from the incoming request should **not** be forwarded - it contains the transformer's hostname, not the predictor's.
- When auth is enabled, the predictor's kube-rbac-proxy validates the bearer token in the `Authorization` header against Kubernetes RBAC.