# Deploy InferenceService with Transformer and PyTorch Runtimes

Please refer to [transformer example doc](https://github.com/kserve/website/tree/main/docs/modelserving/v1beta1/transformer/torchserve_image_transformer) on KServe website repository for deploying PyTorch models with pre-processing in Transformer.

## Testing with a local transformer & predictor in Kubernetes (KServe developer guide)

If you are making changes to the code and would like to test running a transformer on your local machine with the predictor running in Kubernetes,

(1) Follow the above steps to deploy the transformer.yaml to Kubernetes. You can comment out the transformer section in the YAML if you wish to only deploy the predictor to Kubernetes.

(2) Run `pip install -e .` in this directory. If you are using a local version of KServe, you should also run this command in the appropriate directory with a setup.py file so that you pick up any changes from there.

(3) Port-forward the predictor pod by running 
```
kubectl port-forward pods/{name-of-predictor-pod} 8081:8080 
```
Since the predictor pod will expose 8080, pick another port to use with localhost. Here, 8081 is used.

*Note that if you use KServe 0.11 or later, you can skip this step and add `--use_ssl` flag to the command below.*

(4) Use `localhost:8081` as the {predictor-url} to run the following command from this directory. Pass in the `-config_path="local_config.properties"` to use a local config file. If you deploy the transformer to Kubernetes, it will pull the file from GCS.

```bash
python3 -m image_transformer --predictor_host={predictor-url}  --workers=1 --config_path="local_config.properties"
```

With KServe 0.11 or later, you can connect to the predictor host via ssl. 
```bash
python3 -m image_transformer --use_ssl --predictor_host={predictor-url}  --workers=1 --config_path="local_config.properties"
```

If you need to use a custom certificate file, you can set an environment variable.

1. For GRPC, `export GRPC_DEFAULT_SSL_ROOTS_FILE_PATH=tls-ca-bundle.pem`. You can find more gRPC environment variables  [here](https://github.com/grpc/grpc/blob/0526a51734003180862331e2527503a5d1898c95/doc/environment_variables.md).
2. For REST, `export SSL_CERT_FILE=tls-ca-bundle.pem`.


Now your service is running and you can send a request via localhost! 

```bash
>> curl localhost:8080/v1/models/mnist:predict --data @./input.json
{"predictions": [2]}
```
