# KServe on Kubeflow with Istio-Dex

This example shows how to create an InferenceService as well as sending a prediction request to the InferenceService in an Istio-Dex environment.

We will be using the [SKLearn example](/docs/samples/v1beta1/sklearn/v1) to create our InferenceService.

## Setup
Deploy a Multi-user, auth-enabled Kubeflow from [Kustomize manifests](https://github.com/kubeflow/manifests#installation).

## Create the InferenceService

Create the InferenceService using the [manifest](sklearn.yaml) in your namespace (this example uses the namespace `kubeflow-user-example-com`)

```bash
kubectl apply -f sklearn.yaml -n kubeflow-user-example-com
```

Expected Output
```
$ inferenceservice.serving.kserve.io/sklearn-iris created
```

## Run a prediction

### Authentication 
There are 2 ways to authenticate with kubeflow in order to send prediction requests to the InferenceService.

#### Authenticate through service account token
By default, kubeflow creates two `ServiceAccount` namely `default-editor` and `default-viewer` for every user in their kubeflow user namespace.
We are going to use the `default-editor` ServiceAccount to create a JWT token which allows us to authenticate with dex.
```bash
   kubectl get sa -n kubeflow-user-example-com
```
1. Create a JWT token for the `ServiceAccount` with audience `istio-ingressgateway.istio-system.svc.cluster.local`.
   Modify the duration according to your needs.
   ```shell
    TOKEN=$(kubectl create token default-editor -n kubeflow-user-example-com --audience=istio-ingressgateway.istio-system.svc.cluster.local --duration=24h)
   ```
#### Prediction
```bash
curl -v -H "Host: sklearn-iris.kubeflow-user-example-com.example.com" -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d @./iris-input.json http://localhost:8080/v1/models/sklearn-iris:predict
```

**Expected Output**
```bash
*   Trying 127.0.0.1:8080...
* Connected to localhost (127.0.0.1) port 8080 (#0)
> POST /v1/models/sklearn-iris:predict HTTP/1.1
> Host: sklearn-iris.kubeflow-user-example-com.example.com
> User-Agent: curl/7.85.0
> Accept: */*
> Authorization: Bearer eyJhbGciOiJSUzI1NiIsImtpZCI6IkY2S29nTmpNOGZmUkZiZnh2d2Q0QzJfNWc5UjIybF9FTXZ2ZXpORzM3VjQifQ.eyJhdWQiOlsiaXN0aW8taW5ncmVzc2dhdGV3YXkuaXN0aW8tc3lzdGVtLnN2Yy5jbHVzdGVyLmxvY2FsIl0sImV4cCI6MTcwMTQxOTYwNywiaWF0IjoxNzAxMzMzMjA3LCJpc3MiOiJodHRwczovL2t1YmVybmV0ZXMuZGVmYXVsdC5zdmMuY2x1c3Rlci5sb2NhbCIsImt1YmVybmV0ZXMuaW8iOnsibmFtZXNwYWNlIjoia3ViZWZsb3ctdXNlci1leGFtcGxlLWNvbSIsInNlcnZpY2VhY2NvdW50Ijp7Im5hbWUiOiJleHRlcm5hbC1jbGllbnQiLCJ1aWQiOiJhYTM5MTE1Mi03NjMzLTQyYTgtOWI3My03NmJkYmUzYjY0YTAifX0sIm5iZiI6MTcwMTMzMzIwNywic3ViIjoic3lzdGVtOnNlcnZpY2VhY2NvdW50Omt1YmVmbG93LXVzZXItZXhhbXBsZS1jb206ZXh0ZXJuYWwtY2xpZW50In0.nmk7AnNN8x2MPW_x77yk8wAFwgyiyHWzYfM0F1FpdZvGoG5Xl_lEr_7wIE4PuYm0Y5ATSC6ug5BbGlPg6fgvARcSQZYqq8T6ha3YtXrtt7V_VJnGYjC3bIM1mKW7aiQBzhoTjecRAy4iOrdpMK4UNVgxEIYxTl-KNdaFStgrYY90u3yY51_birTSJBzwF4pdSVkJeR8YHvUeb3yTYGeTpab23NhcspeUoXXtSp2cqqaTS-LJd0JU_5y45DqzMmMGt4s2XWsuOsUl3AQ8iFqGKFfsBWaBp9cReETooGKcQN9dtOfOTZ9K-Kvr93oo5aycr7EzzPlFqapFP-9n7ueXQw
> Content-Type: application/json
> Content-Length: 76
> 
* Mark bundle as not supporting multiuse
< HTTP/1.1 200 OK
< content-length: 21
< content-type: application/json
< date: Thu, 30 Nov 2023 08:48:50 GMT
< server: istio-envoy
< x-envoy-upstream-service-time: 72
< 
* Connection #0 to host localhost left intact
{"predictions":[1,1]}
```

#### Authenticate through cookies

The below python code defines a *get_istio_auth_session()* function that returns a session cookie by authenticating with dex.

```python
import re
from urllib.parse import urlsplit
import requests

def get_istio_auth_session(url: str, username: str, password: str) -> dict:
    """
    Determine if the specified URL is secured by Dex and try to obtain a session cookie.
    WARNING: only Dex `staticPasswords` and `LDAP` authentication are currently supported
             (we default to using `staticPasswords` if both are enabled)

    :param url: Kubeflow server URL, including protocol
    :param username: Dex `staticPasswords` or `LDAP` username
    :param password: Dex `staticPasswords` or `LDAP` password
    :return: auth session information
    """
    # define the default return object
    auth_session = {
        "endpoint_url": url,  # KF endpoint URL
        "redirect_url": None,  # KF redirect URL, if applicable
        "dex_login_url": None,  # Dex login URL (for POST of credentials)
        "is_secured": None,  # True if KF endpoint is secured
        "session_cookie": None,  # Resulting session cookies in the form "key1=value1; key2=value2"
    }

    # use a persistent session (for cookies)
    with requests.Session() as s:
        ################
        # Determine if Endpoint is Secured
        ################
        resp = s.get(url, allow_redirects=True)
        if resp.status_code != 200:
            raise RuntimeError(
                f"HTTP status code '{resp.status_code}' for GET against: {url}"
            )

        auth_session["redirect_url"] = resp.url

        # if we were NOT redirected, then the endpoint is UNSECURED
        if len(resp.history) == 0:
            auth_session["is_secured"] = False
            return auth_session
        else:
            auth_session["is_secured"] = True

        ################
        # Get Dex Login URL
        ################
        redirect_url_obj = urlsplit(auth_session["redirect_url"])

        # if we are at `/auth?=xxxx` path, we need to select an auth type
        if re.search(r"/auth$", redirect_url_obj.path):
            #######
            # TIP: choose the default auth type by including ONE of the following
            #######

            # OPTION 1: set "staticPasswords" as default auth type
            redirect_url_obj = redirect_url_obj._replace(
                path=re.sub(r"/auth$", "/auth/local", redirect_url_obj.path)
            )
            # OPTION 2: set "ldap" as default auth type
            # redirect_url_obj = redirect_url_obj._replace(
            #     path=re.sub(r"/auth$", "/auth/ldap", redirect_url_obj.path)
            # )

        # if we are at `/auth/xxxx/login` path, then no further action is needed (we can use it for login POST)
        if re.search(r"/auth/.*/login$", redirect_url_obj.path):
            auth_session["dex_login_url"] = redirect_url_obj.geturl()

        # else, we need to be redirected to the actual login page
        else:
            # this GET should redirect us to the `/auth/xxxx/login` path
            resp = s.get(redirect_url_obj.geturl(), allow_redirects=True)
            if resp.status_code != 200:
                raise RuntimeError(
                    f"HTTP status code '{resp.status_code}' for GET against: {redirect_url_obj.geturl()}"
                )

            # set the login url
            auth_session["dex_login_url"] = resp.url

        ################
        # Attempt Dex Login
        ################
        resp = s.post(
            auth_session["dex_login_url"],
            data={"login": username, "password": password},
            allow_redirects=True,
        )
        if len(resp.history) == 0:
            raise RuntimeError(
                f"Login credentials were probably invalid - "
                f"No redirect after POST to: {auth_session['dex_login_url']}"
            )

        # store the session cookies in a "key1=value1; key2=value2" string
        auth_session["session_cookie"] = "; ".join(
            [f"{c.name}={c.value}" for c in s.cookies]
        )
        auth_session["authservice_session"] = s.cookies.get("authservice_session")

    return auth_session
```

#### Prediction

This python code uses the above function to obtain the authservice_session token in order to 
send authenticated prediction requests to the `InferenceService`.

```python
import requests 

KUBEFLOW_ENDPOINT = "http://localhost:8080"   # Cluster IP and port
KUBEFLOW_USERNAME = "user@example.com"
KUBEFLOW_PASSWORD = "12341234"
MODEL_NAME = "sklearn-iris"
SERVICE_HOSTNAME = "sklearn-iris.kubeflow-user-example-com.example.com"
PREDICT_ENDPOINT = f"{KUBEFLOW_ENDPOINT}/v1/models/{MODEL_NAME}:predict"
iris_input = {"instances": [[6.8, 2.8, 4.8, 1.4], [6.0, 3.4, 4.5, 1.6]]}

_auth_session = get_istio_auth_session(
    url=KUBEFLOW_ENDPOINT, username=KUBEFLOW_USERNAME, password=KUBEFLOW_PASSWORD
)

cookies = {"authservice_session": _auth_session['authservice_session']}
jar = requests.cookies.cookiejar_from_dict(cookies)

res = requests.post(
    url=PREDICT_ENDPOINT,
    headers={"Host": SERVICE_HOSTNAME, "Content-Type": "application/json"},
    cookies=jar,
    json=iris_input,
    timeout=200,
)
print("Status Code: ", res.status_code)
print("Response: ", res.json())
```
> **NOTE:**
> Change the Variables as necessary. 
> 
> You can get the cluster IP by executing the following command.
> ```bash
> kubectl -n istio-system get service istio-ingressgateway -o jsonpath='{.spec.clusterIP}'
> ```
> 
> You can get the `SERVICE_HOSTNAME` by executing the following command.
> ```bash
> kubectl get -n kubeflow-user-example-com inferenceservice ${MODEL_NAME} -o jsonpath='{.status.url}' | cut -d "/" -f 3
> ```

### Expected Output

```bash
Status Code: 200
Response: {"predictions": [1, 1]}
```

### FAQ

1. Why I am getting 404 not found error ?

   Check the url and service hostname for typos. If you are sending request from outside the cluster and service 
   hostname ends with `svc.cluster.local` domain, you need to [configure the domain/DNS](https://knative.dev/docs/install/yaml-install/serving/install-serving-with-yaml/#configure-dns)
   in knative serving.

2. Why I am getting 403 Forbidden RBAC access denied error ?
   
   If you are using istio sidecar injection then, create an [Istio AuthorizationPolicy](https://istio.io/latest/docs/reference/config/security/authorization-policy/) to grant access to the pods or 
   disable it by adding the annotation `sidecar.istio.io/inject: false` to the InferenceService.
