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

echo "::group::Kserve Controller Logs"
kubectl logs -l control-plane=kserve-controller-manager -n kserve -c manager --tail -1
echo "::endgroup::"

echo "::group::Predictor Pod logs"
# Get only pods that are not being deleted.
predictor_pods=$(kubectl get pods -l 'component in (predictor)' -o json -n kserve-ci-e2e-test | jq -r '.items[] | select(.metadata.deletionTimestamp == null) | .metadata.name')
for pod in $predictor_pods; do
    echo "=====================================  Logs for Predictor Pod: $pod  ========================================="
    kubectl logs "$pod" -c kserve-container -n kserve-ci-e2e-test --tail 500
    echo "================================================================================================================"
done
echo "::endgroup::"

echo "::group::Transformer Pod logs"
# Get only pods that are not being deleted.
transformer_pods=$(kubectl get pods -l 'component in (transformer)' -o json -n kserve-ci-e2e-test | jq -r '.items[] | select(.metadata.deletionTimestamp == null) | .metadata.name')
for pod in $transformer_pods; do
    echo "=====================================  Logs for Transformer Pod: $pod  ======================================="
    kubectl logs "$pod" -c kserve-container -n kserve-ci-e2e-test --tail 500
    echo "================================================================================================================"
done
echo "::endgroup::"

echo "::group::Explainer Pod logs"
# Get only pods that are not being deleted.
explainer_pods=$(kubectl get pods -l 'component in (explainer)' -o json -n kserve-ci-e2e-test | jq -r '.items[] | select(.metadata.deletionTimestamp == null) | .metadata.name')
for pod in $explainer_pods; do
    echo "=====================================  Logs for Explainer Pod: $pod  ========================================="
    kubectl logs "$pod" -c kserve-container -n kserve-ci-e2e-test --tail 500
    echo "================================================================================================================"
done
echo "::endgroup::"

echo "::group::InferenceGraph Pod logs"
# Get only pods that are not being deleted.
graph_pods=$(kubectl get pods -l 'serving.kserve.io/inferencegraph=model-chainer' -o json -n kserve-ci-e2e-test | jq -r '.items[] | select(.metadata.deletionTimestamp == null) | .metadata.name')
for pod in $graph_pods; do
    echo "=====================================  Logs for Graph Pod: $pod  ========================================="
    kubectl logs "$pod" -c user-container -n kserve-ci-e2e-test --tail 500
    echo "================================================================================================================"
done
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
