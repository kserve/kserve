# Customized URLs Sample

In some situations, we want to deploy the inference service under `custom model`, and we just have the custom image, can not modify it's server route, this server maybe have one more endpoints. For these situations, we can use adding an annotation to solve this problem.

## A simple sample 

* `app.py` is a simple flask server, it has tow endpoints: `customized_urls_test` and `hello_world`

```yaml
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

  Run the build command to build your image. Or you can use the the prebuild image:`iamlovingit/customized-urls:latest` 

  ```shell
  docker build -t ${your-dockerhub-id}/customized-urls:latest .
  docker push ${your-dockerhub-id}/customized-urls:latest
  ```

* Config the Yaml

  If you have more than one endpoints in your server, you can use `,` to split them.

  ```yaml
  apiVersion: serving.kubeflow.org/v1alpha2
  kind: InferenceService
  metadata:
    labels:
      controller-tools.k8s.io: "1.0"
    name: customized-urls-sample
    annotations:
      custom.urls: "/customized/urls/demo/test/1,/v1/models/customized-sample:predict"
  spec:
    default:
      predictor:
        custom:
          container:
            image: iamlovingit/customized-urls:latest
            container_port: 5000
  ```

* Deploy the  yaml and check the Inference service status

  ```shel
  kubectl apply -f customized-urls.yaml
  kubectl get inferenceservice customized-urls-sample
  ```

  Expect output:

  ```shell
  NAME                     URL                                                                               
                                                 READY   DEFAULT TRAFFIC   CANARY TRAFFIC   AGE
  customized-urls-sample   http://customized-urls-sample.default.10.166.15.200.xip.io/customized/urls/demo/test/1,http://customized-urls-sample.default.10.166.15.200.xip.io/v1/models/customized-sample:predict   True    100                                17m
  ```

  There will display two URLs for this service.

  ```
  http://customized-urls-sample.default.10.166.15.200.xip.io/customized/urls/demo/test/1
  http://customized-urls-sample.default.10.166.15.200.xip.io/v1/models/customized-sample:predict
  ```

  Test the endpoints:

  `curl http://customized-urls-sample.default.10.166.15.200.xip.io/customized/urls/demo/test/1`

  Expect Output: `Hello World. Func customized_urls_test is called!`
  `curl http://customized-urls-sample.default.10.166.15.200.xip.io/v1/models/customized-sample:predict`
  Expect Output: `Hello World!`

  


