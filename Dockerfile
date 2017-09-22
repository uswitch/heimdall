FROM scratch

ADD bin/heimdall heimdall
ADD controller/templates templates

ENTRYPOINT ["/heimdall"]
