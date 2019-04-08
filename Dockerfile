# Build the manager binary
FROM golang:1.10.3 as builder

# Copy in the go src
WORKDIR /go/src/github.com/kubeflow/kfserving
COPY pkg/    pkg/
COPY cmd/    cmd/
COPY config/ config/
COPY vendor/ vendor/
COPY hack/ hack/
COPY Makefile Makefile
COPY PROJECT PROJECT
ARG BIN=/usr/local/kubebuilder/bin
ARG version=1.0.8
ARG os=linux
ARG arch=amd64
RUN mkdir -p $BIN
RUN wget -q -O $BIN/kube-apiserver https://artprod.dev.bloomberg.com/artifactory/libs-snapshot/com/bloomberg/ds/kubebuilder/releases/kube-apiserver
RUN chmod +x $BIN/kube-apiserver
RUN wget -q -O $BIN/etcd https://artprod.dev.bloomberg.com/artifactory/libs-snapshot/com/bloomberg/ds/kubebuilder/releases/etcd
RUN chmod +x $BIN/etcd
RUN ls -ltr $BIN 
#RUN curl -L -O "https://github.com/kubernetes-sigs/kubebuilder/releases/download/v${version}/kubebuilder_${version}_${os}_${arch}.tar.gz"
#RUN tar -zxvf kubebuilder_${version}_${os}_${arch}.tar.gz
#RUN mv kubebuilder_${version}_${os}_${arch} kubebuilder && mv kubebuilder /usr/local
RUN export PATH=$PATH:/usr/local/kubebuilder/bin
RUN make test
# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o manager ./cmd/manager

# Copy the controller-manager into a thin image
FROM ubuntu:latest
WORKDIR /
COPY --from=builder /go/src/kfserving/manager .
ENTRYPOINT ["/manager"]
