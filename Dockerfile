FROM golang:1.12.1-alpine3.9 as debug
ENV CGO_ENABLED 0
WORKDIR /
ADD bin/heimdall heimdall
RUN set -ex; \
    apk add --no-cache \
        git \
    ; \
    go get github.com/derekparker/delve/cmd/dlv;

ENTRYPOINT ["/go/bin/dlv", "--listen=:40000", "--headless=true", "--api-version=2", "exec", "/heimdall", "--"]

CMD ["--json"]

FROM scratch as release
ADD bin/heimdall heimdall

ENTRYPOINT ["/heimdall"]

CMD ["--json"]
