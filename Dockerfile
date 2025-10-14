FROM gcr.io/distroless/static:nonroot

WORKDIR /

COPY --chmod=755 bin/heimdall-linux-amd64 heimdall

USER nonroot:nonroot

ENTRYPOINT ["/heimdall"]
