# Local Test of Kfserving Logger

In one terminal start an echo http server on port 8000

```
docker run -it -p 8000:80 --rm -t mendhak/http-https-echo
```

Start an SKLearn Iris model on port 8080. You will need to have pip installed the sklearnserver. See `/python/sklearnserver`.

```
gsutil cp -r gs://kfserving-samples/models/sklearn/iris .
LOCAL_DIR=$(pwd)/iris
python -m sklearnserver --model_dir $LOCAL_DIR --model_name sklearn-iris --http_port 8080
```

Start the Kfserving logger from Kfserving root folder:

```

bin/logger --log-url http://0.0.0.0:8000 --component-port 8080 --log-mode all --model-id=iris --namespace=default --predictor=default
```

Send a request:

```
curl -v -d @./input.json http://0.0.0.0:8081/v1/models/sklearn-iris:predict
```

You should see output like:

```
*   Trying 0.0.0.0...
* Connected to 0.0.0.0 (127.0.0.1) port 8081 (#0)
> POST /v1/models/sklearn-iris:predict HTTP/1.1
> Host: 0.0.0.0:8081
> User-Agent: curl/7.47.0
> Accept: */*
> Content-Length: 76
> Content-Type: application/x-www-form-urlencoded
>
* upload completely sent off: 76 out of 76 bytes
< HTTP/1.1 200 OK
< Content-Type: text/html; charset=UTF-8
< Date: Fri, 20 Dec 2019 18:23:49 GMT
< Content-Length: 23
<
* Connection #0 to host 0.0.0.0 left intact
{"predictions": [1, 1]}
```

This shows the prediction has worked. In the output of the http-https-echo server you should see the request and response payloads echoed.


```
{ path: '/',
  headers: 
   { host: '0.0.0.0:8000',
     'user-agent': 'Go-http-client/1.1',
     'content-length': '76',
     'ce-cloudeventsversion': '0.1',
     'ce-eventid': '29232038-6c2a-44b3-a542-95c499732ec0',
     'ce-eventtime': '2019-10-31T15:20:50.435513493Z',
     'ce-eventtype': 'org.kubeflow.serving.inference.request',
     'ce-source': 'http://localhost:8080/',
     'content-type': 'application/json',
     'kf-model-uri': '',
     'accept-encoding': 'gzip' },
  method: 'POST',
  body: '{  "instances": [    [6.8,  2.8,  4.8,  1.4],    [6.0,  3.4,  4.5,  1.6]  ]}',
  cookies: undefined,
  fresh: false,
  hostname: '0.0.0.0',
  ip: '::ffff:172.17.0.1',
  ips: [],
  protocol: 'http',
  query: {},
  subdomains: [],
  xhr: false,
  os: { hostname: 'a167987d8875' } }
::ffff:172.17.0.1 - - [31/Oct/2019:15:20:50 +0000] "POST / HTTP/1.1" 200 796 "-" "Go-http-client/1.1"
-----------------
{ path: '/',
  headers: 
   { host: '0.0.0.0:8000',
     'user-agent': 'Go-http-client/1.1',
     'content-length': '23',
     'ce-cloudeventsversion': '0.1',
     'ce-eventid': '29232038-6c2a-44b3-a542-95c499732ec0',
     'ce-eventtime': '2019-10-31T15:20:50.438892641Z',
     'ce-eventtype': 'org.kubeflow.serving.inference.response',
     'ce-source': 'http://localhost:8080/',
     'content-type': 'application/json; charset=UTF-8',
     'kf-model-uri': '',
     'accept-encoding': 'gzip' },
  method: 'POST',
  body: '{"predictions": [1, 1]}',
  cookies: undefined,
  fresh: false,
  hostname: '0.0.0.0',
  ip: '::ffff:172.17.0.1',
  ips: [],
  protocol: 'http',
  query: {},
  subdomains: [],
  xhr: false,
  os: { hostname: 'a167987d8875' } }
::ffff:172.17.0.1 - - [31/Oct/2019:15:20:50 +0000] "POST / HTTP/1.1" 200 759 "-" "Go-http-client/1.1"
```

