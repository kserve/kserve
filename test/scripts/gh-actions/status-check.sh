#!/bin/bash

sleep 10
echo "::group::Free Space"
df -hT
echo "::endgroup::"

echo "::group::List Docker Images"
docker image ls
echo "::endgroup::"

echo "::group::List Pods in kserve namespace"
kubectl get pods -n kserve
echo "::endgroup::"

echo "::group::K8s Events in kserve namespace"
kubectl get events -n kserve
echo "::endgroup::"

echo "::group::Describe pods/Gather logs in kserve namespace"
if ! kubectl get namespace kserve &>/dev/null; then
  echo "⚠️ Namespace kserve does not exist, skipping..."
  return
fi

for pod in $(kubectl get pods -n  kserve -o jsonpath='{.items[*].metadata.name}' 2>/dev/null); do
  echo "--- Pod: $pod ---"
  kubectl describe pods -n kserve $pod
  kubectl logs -n kserve $pod --all-containers=true --tail=1000 2>&1
  echo "--- End Pod: $pod ---"
done
echo "::endgroup::"

echo "::group::Pod manifest in kserve namespace"
if ! kubectl get namespace kserve &>/dev/null; then
  echo "⚠️ Namespace kserve does not exist, skipping..."
  return
fi

for pod in $(kubectl get pods -n  kserve -o jsonpath='{.items[*].metadata.name}' 2>/dev/null); do
  echo "--- Pod: $pod ---"
  kubectl get pods -n kserve $pod -o yaml
  echo "--- End Pod: $pod ---"
done
echo "::endgroup::"

echo "::group::List Pods in keda namespace"
kubectl get pods -n keda
echo "::endgroup::"

echo "::group::List Pods in all other namespaces"
kubectl get pods -A --field-selector=metadata.namespace!=kserve,metadata.namespace!=kserve-ci-e2e-test
echo "::endgroup::"

echo "::group::List Pods in kserve-ci-e2e-test namespace"
kubectl get pods -n kserve-ci-e2e-test
echo "::endgroup::"

echo "::group::Describe Pods in kserve-ci-e2e-test namespace"
kubectl describe pods -n kserve-ci-e2e-test
echo "::endgroup::"

echo "::group::K8s Events in kserve-ci-e2e-test namespace"
kubectl get events -n kserve-ci-e2e-test
echo "::endgroup::"

echo "::group::Gather logs in kserve-ci-e2e-test namespace"
if ! kubectl get namespace kserve-ci-e2e-test &>/dev/null; then
  echo "⚠️ Namespace kserve-ci-e2e-test does not exist, skipping..."
  return
fi

for pod in $(kubectl get pods -n  kserve-ci-e2e-test -o jsonpath='{.items[*].metadata.name}' 2>/dev/null); do
  echo "--- Pod: $pod ---"
  kubectl describe pods -n kserve-ci-e2e-test $pod
  kubectl logs -n kserve-ci-e2e-test $pod --all-containers=true --tail=1000 2>&1
  echo "--- End Pod: $pod ---"
done
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

echo "::group::KEDA Pod logs"
for pod in $(kubectl get pods -o jsonpath='{.items[*].metadata.name}' -n keda); do
    echo "=====================================  Logs for KEDA Pod: $pod  ========================================="
    kubectl logs "$pod" -n keda --tail 500
    echo "================================================================================================================"
done
echo "::endgroup::"

echo "::group::OpenTelemetry Operator Pod logs"
for pod in $(kubectl get pods -o jsonpath='{.items[*].metadata.name}' -n opentelemetry-operator); do
    echo "=====================================  Logs for OpenTelemetry Operator Pod: $pod  ========================================="
    kubectl logs "$pod" -n opentelemetry-operator --tail 500
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

if [[ $# -eq 1 && "$1" == "llmisvc" ]]; then
  echo "::group::Enhanced LLMISvc system status check... Resources"
  kubectl get gateways -A -o yaml
  kubectl get httproutes -A
  kubectl get httproute -n kserve-ci-e2e-test -o yaml
  kubectl get inferencepools -A
  kubectl get inferenceobjectives -A
  kubectl get inferenceobjectives -n kserve-ci-e2e-test -o yaml
  kubectl get inferencepools -n kserve-ci-e2e-test -o yaml
  kubectl get llminferenceservices -n kserve-ci-e2e-test -o yaml
  kubectl get llminferenceserviceconfigs -A
  kubectl get validatingwebhookconfiguration | grep llm
  kubectl get gatewayclasses -A
  kubectl get svc -A
  kubectl get certificate -A
  echo "::endgroup::"
  echo "::group::Describing LLMInferenceServices in kserve-ci-e2e-test namespace"
  if ! kubectl get namespace kserve-ci-e2e-test &>/dev/null; then
    echo "⚠️ Namespace kserve-ci-e2e-test does not exist, skipping..."
    return
  fi
  
  for llmisvc in $(kubectl get llminferenceservices -n kserve-ci-e2e-test -o jsonpath='{.items[*].metadata.name}' 2>/dev/null); do
    echo "=== LLMInferenceService: $llmisvc ==="
    kubectl describe llminferenceservices -n kserve-ci-e2e-test $llmisvc 2>&1
  done

  echo "::endgroup::"

  echo "::group::Gather logs in envoy-gateway-system envoy-ai-gateway-system"
  NAMESPACES="envoy-gateway-system envoy-ai-gateway-system"

  for ns in $NAMESPACES; do
    if ! kubectl get namespace $ns &>/dev/null; then
      echo "⚠️ Namespace $ns does not exist, skipping..."
      continue
    fi

    echo "=== Namespace: $ns ==="

    echo "--- Events ---"
    kubectl get events -n $ns --sort-by='.lastTimestamp' | tail -20
    echo "--- End Events ---"

    echo "--- Pods ---"
    kubectl get pods -n $ns
    for pod in $(kubectl get pods -n $ns -o jsonpath='{.items[*].metadata.name}' 2>/dev/null); do
      echo "--- Pod: $pod ---"
      kubectl describe pods -n $ns $pod
      kubectl logs -n $ns $pod --all-containers=true --tail=1000 2>&1
      echo "--- End Pod: $pod ---"
    done
    echo "--- End Pods ---"
  done
  echo "::endgroup::"  
fi
shopt -u nocasematch
