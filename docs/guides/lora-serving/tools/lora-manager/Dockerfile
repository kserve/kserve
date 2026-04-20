FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

RUN microdnf install -y jq && \
    microdnf clean all

COPY lora-manager.sh /usr/local/bin/lora-manager
RUN chmod +x /usr/local/bin/lora-manager

ENTRYPOINT ["lora-manager"]
