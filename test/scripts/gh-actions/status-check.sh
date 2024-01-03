#!/bin/bash

sleep 10
echo "::group::Free Space"
df -T
echo "::endgroup::"

echo "::group::List Docker Images"
docker image ls
echo "::endgroup::"

echo "::group::List Pods in kserve and kserve-ci-e2e-test namespace"
kubectl get pods -n kserve
kubectl get pods -n kserve-ci-e2e-test
echo "::endgroup::"

echo "::group::List Pods in all other namespaces"
kubectl get pods -A --field-selector=metadata.namespace!=kserve,metadata.namespace!=kserve-ci-e2e-test
echo "::endgroup::"

echo "::group::Describe Pods in kserve-ci-e2e-test namespace"
kubectl describe pods -n kserve-ci-e2e-test
echo "::endgroup::"

echo "::group::K8s Events in kserve-ci-e2e-test namespace"
kubectl get events -n kserve-ci-e2e-test
echo "::endgroup::"

echo "::group::Kserve Controller Logs"
kubectl logs -l control-plane=kserve-controller-manager -n kserve -c manager --tail -1
echo "::endgroup::"

echo "::group::Predictor Pod logs"
kubectl logs -l 'component in (predictor)' -c kserve-container -n kserve-ci-e2e-test --tail 500
echo "::endgroup::"

echo "::group::Transformer Pod logs"
kubectl logs -l 'component in (transformer)' -c kserve-container -n kserve-ci-e2e-test --tail 500
echo "::endgroup::"

echo "::group::Explainer Pod logs"
kubectl logs -l 'component in (explainer)' -c kserve-container -n kserve-ci-e2e-test --tail 500
echo "::endgroup::"

shopt -s nocasematch
if [[ $# -eq 1 && "$1" == "kourier" ]]; then
  echo "::group::Kourier Gateway Pod logs"
  kubectl logs "$(kubectl get pod -n knative-serving -l 'app=3scale-kourier-gateway' --output=jsonpath='{.items[0].metadata.name}')" -n knative-serving
  echo "::endgroup::"
else
  echo "::group::Istio Ingress Gateway Pod logs"
  kubectl logs "$(kubectl get pods -n istio-system --output=jsonpath={.items..metadata.name} -l app=istio-ingressgateway)" -n istio-system
  echo "::endgroup::"
fi
shopt -u nocasematch
