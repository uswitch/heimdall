FROM gcr.io/distroless/static:nonroot

WORKDIR /

COPY bin/heimdall-linux-amd64 org-api

USER nonroot:nonroot

ENTRYPOINT ["/heimdall"]
