# Build the manager binary
FROM golang:1.25 AS deps

WORKDIR /go/src/github.com/kserve/kserve
COPY go.mod go.mod
COPY go.sum go.sum
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# ---- Build stage (parallel with license on BuildKit) ----
FROM deps AS builder

ARG CMD
ARG GOTAGS=""
COPY cmd/${CMD}/ cmd/${CMD}/
COPY pkg/ pkg/
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOFLAGS=-mod=readonly go build -a -tags "${GOTAGS}" -o /out/binary ./cmd/${CMD}

# ---- License stage (parallel with build on BuildKit) ----
FROM deps AS license

ARG CMD
COPY cmd/${CMD}/ cmd/${CMD}/
COPY pkg/ pkg/
COPY LICENSE LICENSE
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go install github.com/google/go-licenses@v1.6.0 \
    && go-licenses check ./cmd/${CMD} ./pkg/... --disallowed_types="forbidden,unknown" \
    && go-licenses save --save_path /third_party/library ./cmd/${CMD}


FROM gcr.io/distroless/static:nonroot AS kserve-controller
COPY --from=license /third_party /third_party
COPY --from=builder /out/binary /manager
ENTRYPOINT ["/manager"]

FROM gcr.io/distroless/static:nonroot AS llmisvc-controller
COPY --from=license /third_party /third_party
COPY --from=builder /out/binary /manager
ENTRYPOINT ["/manager"]

FROM gcr.io/distroless/static:nonroot AS kserve-localmodel-controller
COPY --from=license /third_party /third_party
COPY --from=builder /out/binary /manager
ENTRYPOINT ["/manager"]

FROM gcr.io/distroless/static:nonroot AS kserve-localmodelnode-agent
COPY --from=license /third_party /third_party
COPY --from=builder /out/binary /manager
ENTRYPOINT ["/manager"]

FROM gcr.io/distroless/static:nonroot AS agent
COPY --from=license /third_party /third_party
COPY --from=builder /out/binary /ko-app/agent
ENTRYPOINT ["/ko-app/agent"]

FROM gcr.io/distroless/static:nonroot AS router
COPY --from=license /third_party /third_party
COPY --from=builder /out/binary /ko-app/router
ENTRYPOINT ["/ko-app/router"]
