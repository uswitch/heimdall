FROM golang:1.12.1-alpine3.9 as debug

RUN set -ex; \
    apk add --no-cache \
        git \
    ; \
    CGO_ENABLED=0 go get github.com/derekparker/delve/cmd/dlv;

WORKDIR /
COPY bin/heimdall heimdall

ENTRYPOINT ["/go/bin/dlv", "--listen=:40000", "--headless=true", "--api-version=2", "exec", "/heimdall", "--"]

CMD ["--json"]

FROM scratch as release
COPY bin/heimdall heimdall

ENTRYPOINT ["/heimdall"]

CMD ["--json"]
