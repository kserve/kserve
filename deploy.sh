cd config/runtimes
kustomize build . -o output.yaml
kubectl apply -f output.yaml
cd ../..
make deploy-dev-djlserving
cd config/runtimes
