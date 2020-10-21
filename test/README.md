# Test

This directory contains tests and testing docs for `KFServing`:

- [Unit tests](#running-unit-tests) currently reside in the codebase alongside
  the code they test
- [End-to-end tests](#running-end-to-end-tests):
  - They are in [`/test/e2e`](./e2e)

## Prerequisite
`kfserving-controller-manager` has a few integration tests which requires mock apiserver
and etcd, they get installed along with [`kubebuilder`](https://book.kubebuilder.io/quick-start.html#installation).

## Running unit/integration tests

To run all unit tests:

```bash
make test
```

## Running end to end tests

To run [the e2e tests](./e2e), you
need to have a running environment that meets the e2e test environment requirements.

First have kfserving installed in a cluster.

To setup from local code, do:

 1. `./hack/quick_install.sh`
 2. `make undeploy`
 3. `make deploy-dev`


Install pytest and test deps:
```
pip3 install pytest==6.0.2 pytest-xdist pytest-rerunfailures
pip3 install --upgrade pytest-tornasync
pip3 install urllib3==1.24.2
pip3 install --upgrade setuptools
```

Go to `python/kfserving` and install kfserving deps 
```
pip3 install -r requirements.txt
python3 setup.py install --force --user
```
Then go to `test/e2e`. 

Run `kubectl create namespace kfserving-ci-e2e-test`

For KIND/minikube:

* Run `export KFSERVING_INGRESS_HOST_PORT=localhost:8080`
* In a different window run `kubectl port-forward -n istio-system svc/istio-ingressgateway 8080:80`
* Note that not all tests will pass as the pytorch test requires gpu. These will show as pending pods at the end.

Run `pytest > testresults.txt`

Tests may not clean up. To re-run, first do `kubectl delete namespace kfserving-ci-e2e-test`, recreate namespace and run again.

Optionally for more detailed info, in another window do `kubectl get pod -n kfserving-ci-e2e-test -w > podwatch.txt`