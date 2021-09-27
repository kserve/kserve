FROM alpine:latest

RUN apk add --no-cache wget bash curl jq

RUN curl -LO https://storage.googleapis.com/kubernetes-release/release/$(curl -s https://storage.googleapis.com/kubernetes-release/release/stable.txt)/bin/linux/amd64/kubectl
RUN chmod +x ./kubectl
RUN mv ./kubectl /usr/local/bin

RUN wget https://github.com/mikefarah/yq/releases/download/3.4.1/yq_linux_amd64 -O /usr/bin/yq &&\
    chmod +x /usr/bin/yq

COPY kserve_migration.sh /kserve_migration.sh

ENTRYPOINT ["./kserve_migration.sh"]
