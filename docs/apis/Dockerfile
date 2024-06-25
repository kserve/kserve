FROM golang:1.21

RUN apt-get update && apt-get -y upgrade && apt-get -y install git

RUN git clone https://github.com/ahmetb/gen-crd-api-reference-docs
# replace the version pattern in the Dockerfile until we don't move to golang 1.22
RUN sed -i 's/1.21.0/1.21/g' gen-crd-api-reference-docs/go.mod

WORKDIR gen-crd-api-reference-docs

RUN go build

VOLUME /go/gen-crd-api-reference-docs/apidocs

ENTRYPOINT ["./gen-crd-api-reference-docs"]
CMD ["-config", "./example-config.json", "-api-dir", "../src/github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1", "-out-file", "./apidocs/v1beta1/README.md"]
