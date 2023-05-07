FROM golang:1.20.3 AS build
WORKDIR /
COPY bgtest /project/bgtest/
RUN cd /project/bgtest && go build -mod vendor -o bgtest -ldflags '-linkmode "external" -extldflags "-static"'

FROM scratch AS final
COPY --from=build /project/bgtest/bgtest .
CMD ["./bgtest"]