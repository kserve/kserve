# Predict on a InferenceService with transformer using Torchserve

Please refer to [transformer example on website doc repo](https://github.com/kserve/website/tree/main/docs/modelserving/v1beta1/transformer/torchserve_image_transformer#deploy-the-inferenceservice-with-rest-predictor)

## Testing with a local transformer & predictor in Kubernetes (Kserve developer guide)

If you are making changes to the code and would like to test running a transformer on your local machine with the predictor running in Kubernetes,

(1) Follow the above steps to deploy the transformer.yaml to Kubernetes. You can comment out the transformer section in the YAML if you wish to only deploy the predictor to Kubernetes.

(2) Run `pip install -e .` in this directory. If you are using a local version of Kserve, you should also run this command in the appropriate directory with a setup.py file so that you pick up any changes from there. 

(3) Port-forward the predictor pod by running 
```
kubectl port-forward pods/{name-of-predictor-pod} 8081:8080 
```
Since the predictor pod will expose 8080, pick another port to use with localhost. Here, 8081 is used.

(4) Use `localhost:8081` as the {predictor-url} to run the following command from this directory. Pass in the `-config_path="local_config.properties"` to use a local config file. If you deploy the transformer to Kubernetes, it will pull the file from GCS.

```bash
python3 -m image_transformer --predictor_host={predictor-url}  --workers=1 --config_path="local_config.properties"
```

Now your service is running and you can send a request via localhost! 

```bash
>> curl localhost:8080/v1/models/mnist:predict --data @./input.json
{"predictions": [2]}
```
