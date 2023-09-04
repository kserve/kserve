#!/bin/bash

sleep 10
df -T
docker image ls
kubectl get pods -n kserve
kubectl get pods -n kserve-ci-e2e-test
kubectl get pods -A --field-selector=metadata.namespace!=kserve,metadata.namespace!=kserve-ci-e2e-test

kubectl describe pods -n kserve-ci-e2e-test
kubectl get events -n kserve-ci-e2e-test
kubectl logs -l control-plane=kserve-controller-manager -n kserve -c manager
kubectl logs -l 'component in (predictor)' -c kserve-container -n kserve-ci-e2e-test --tail 500
kubectl logs -l 'component in (transformer)' -c kserve-container -n kserve-ci-e2e-test --tail 500
kubectl logs -l 'component in (explainer)' -c kserve-container -n kserve-ci-e2e-test --tail 500
kubectl logs "$(kubectl get pods -n istio-system --output=jsonpath={.items..metadata.name} -l app=istio-ingressgateway)" -n istio-system

