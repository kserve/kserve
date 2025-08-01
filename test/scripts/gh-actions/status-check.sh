#!/bin/bash

sleep 10
echo "::group::Free Space"
df -hT
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

echo "::group::K8s Events in kserve-localmodel-jobs namespace"
kubectl get events -n kserve-localmodel-jobs
echo "::endgroup::"

echo "::group::Kserve Controller Logs"
kubectl logs -l control-plane=kserve-controller-manager -n kserve -c manager --tail -1
echo "::endgroup::"

echo "::group::Kserve ModelCache Controller Logs"
kubectl logs -l control-plane=kserve-localmodel-controller-manager -n kserve -c manager --tail -1
echo "::endgroup::"

echo "::group::Kserve ModelCache Agent Logs"
for pod in $(kubectl get pods -l control-plane=kserve-localmodelnode-agent -o jsonpath='{.items[*].metadata.name}' -n kserve); do
    echo "=====================================  Logs for modelcache agent: $pod  ========================================="
    kubectl logs "$pod" -c manager -n kserve --tail -1
    echo "================================================================================================================"
done
echo "::endgroup::"

echo "::group::Predictor Pod logs"
for pod in $(kubectl get pods -l 'component in (predictor)' -o jsonpath='{.items[*].metadata.name}' -n kserve-ci-e2e-test); do
    echo "=====================================  Logs for Predictor Pod: $pod  ========================================="
    kubectl logs "$pod" -c kserve-container -n kserve-ci-e2e-test --tail 500
    echo "================================================================================================================"
done
echo "::endgroup::"

echo "::group::Transformer Pod logs"
for pod in $(kubectl get pods -l 'component in (transformer)' -o jsonpath='{.items[*].metadata.name}' -n kserve-ci-e2e-test); do
    echo "=====================================  Logs for Transformer Pod: $pod  ======================================="
    kubectl logs "$pod" -c kserve-container -n kserve-ci-e2e-test --tail 500
    echo "================================================================================================================"
done
echo "::endgroup::"

echo "::group::Explainer Pod logs"
for pod in $(kubectl get pods -l 'component in (explainer)' -o jsonpath='{.items[*].metadata.name}' -n kserve-ci-e2e-test); do
    echo "=====================================  Logs for Explainer Pod: $pod  ========================================="
    kubectl logs "$pod" -c kserve-container -n kserve-ci-e2e-test --tail 500
    echo "================================================================================================================"
done
echo "::endgroup::"

echo "::group::InferenceGraph Pod logs"
for pod in $(kubectl get pods -l 'serving.kserve.io/inferencegraph=model-chainer' -o jsonpath='{.items[*].metadata.name}' -n kserve-ci-e2e-test); do
    echo "=====================================  Logs for Graph Pod: $pod  ========================================="
    kubectl logs "$pod" -c user-container -n kserve-ci-e2e-test --tail 500
    echo "================================================================================================================"
done
echo "::endgroup::"

echo "::group::envoy gateway"
kubectl describe pods -l serving.kserve.io/gateway=kserve-ingress-gateway -n envoy-gateway-system
kubectl describe svc -l gateway.envoyproxy.io/owning-gateway-name=kserve-ingress-gateway -n envoy-gateway-system
echo "::endgroup::"

echo "::group::istio gateway"
kubectl describe pods -l serving.kserve.io/gateway=kserve-ingress-gateway -n kserve
kubectl describe svc -l serving.kserve.io/gateway=kserve-ingress-gateway -n kserve
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
