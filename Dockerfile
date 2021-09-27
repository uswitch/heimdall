FROM gcr.io/distroless/static:nonroot

WORKDIR /

COPY bin/heimdall-linux-amd64 heimdall

USER nonroot:nonroot

ENTRYPOINT ["/heimdall"]
