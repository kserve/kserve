#!/bin/bash

sleep 10
df -T
docker image ls
kubectl get pods -n kserve
kubectl get pods -n kserve-ci-e2e-test

kubectl describe pods -n kserve-ci-e2e-test
kubectl get events -n kserve-ci-e2e-test
kubectl logs -l control-plane=kserve-controller-manager -n kserve -c manager
kubectl logs -l 'component in (predictor, explainer, transformer)' -c kserve-container -n kserve-ci-e2e-test