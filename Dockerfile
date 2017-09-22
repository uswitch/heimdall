FROM scratch

ADD bin/heimdall heimdall

ENTRYPOINT ["/heimdall"]
