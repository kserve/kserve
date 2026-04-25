kubectl port-forward -n openshift-ingress service/openshift-ai-inference-openshift-default 8443:443 &

# Login with an actual user token instead of kubeadmin
TOKEN=$(oc whoami -t)

MODELS=$(curl \
    -k --noproxy localhost -H "Authorization: Bearer $(oc whoami -t)" \
    -H "Content-Type: application/json" \
    https://localhost:8443/greg/qwen2-lora/v1/models)

echo $MODELS | jq '.data[]'


# Request the base model
curl \
    -k --noproxy localhost -H "Authorization: Bearer $(oc whoami -t)" \
    -H "Content-Type: application/json" \
    https://localhost:8443/greg/qwen2-lora/v1/chat/completions \
    -d '{"model":"Qwen/Qwen2.5-7B-Instruct","messages":[{"role":"user","content":"How does the time value of money work"}],"max_tokens":512}' | jq

# Request the adapter
curl \
    -k --noproxy localhost -H "Authorization: Bearer $(oc whoami -t)" \
    -H "Content-Type: application/json" \
    https://localhost:8443/greg/qwen2-lora/v1/chat/completions \
    -d '{"model":"finance-lora","messages":[{"role":"user","content":"How does the time value of money work"}],"max_tokens":512}' | jq


