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

Run `pytest > testresults.txt`