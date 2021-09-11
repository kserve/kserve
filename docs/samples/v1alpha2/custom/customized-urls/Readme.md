# Customized URLs Sample

In some situations, you may have a `custom model image` but can not modify its server routes, this server may also have one or more predict endpoints. Here is a simple sample to show how to deploy this custom service.

## A simple sample 

* `app.py` is a simple flask server, it has two endpoints: `/customized/urls/demo/test/1` and `/v1/models/customized-sample:predict`

```python
import os

from flask import Flask

app = Flask(__name__)
@app.route('/customized/urls/demo/test/1')
def customized_urls_test():
    target = os.environ.get('TARGET', 'World')
    return 'Hello {}. Func customized_urls_test is called!\n'.format(target)

@app.route('/v1/models/customized-sample:predict')
def hello_world():
    target = os.environ.get('TARGET', 'World')
    return 'Hello {}!\n'.format(target)

if __name__ == "__main__":
    app.run(debug=True,host='0.0.0.0',port=int(os.environ.get('PORT', 8080)))
```

* Build custom image.

  Run the build command to build your image.

  ```shell
  docker build -t ${your-dockerhub-id}/customized-urls:latest .
  docker push ${your-dockerhub-id}/customized-urls:latest
  ```

* Config the `InferenceService` yaml

```yaml
apiVersion: serving.kserve.io/v1alpha2
  kind: InferenceService
  metadata:
    name: customized-urls-sample
  spec:
    default:
      predictor:
        custom:
          container:
            name: hello
            image: ${your-dockerhub-id}/customized-urls:latest
```
  
* Deploy the `InferenceService`

```shell
  kubectl apply -f customized-urls.yaml
  kubectl get inferenceservice customized-urls-sample
```

Expect output:

```shell
  NAME                     URL                                                          READY   DEFAULT TRAFFIC   CANARY TRAFFIC   AGE
  customized-urls-sample   http://customized-urls-sample.default.10.166.15.200.xip.io   True    100                                11m
```
  
There will display the service host.
  
```
  http://customized-urls-sample.default.10.166.15.200.xip.io
```
  
Remember your endpoints:

```
  /customized/urls/demo/test/1
  /v1/models/customized-sample:predict
```
  
Append the endpoint after `host`, you can get the route of your endpoint
  
```
  http://customized-urls-sample.default.10.166.15.200.xip.io/customized/urls/demo/test/1
  http://customized-urls-sample.default.10.166.15.200.xip.io/v1/models/customized-sample:predict
```
  
  
  
Let's test the endpoints:
  
`curl http://customized-urls-sample.default.10.166.15.200.xip.io/customized/urls/demo/test/1`
  
Expect Output: `Hello World. Func customized_urls_test is called!`

`curl http://customized-urls-sample.default.10.166.15.200.xip.io/v1/models/customized-sample:predict`

Expect Output: `Hello World!`
  
  
  
Job Done!
  
  
