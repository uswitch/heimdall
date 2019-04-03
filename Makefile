container-image-release:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/heimdall
	docker build --target release -t heimdall .

container-image-debug:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -gcflags "all=-N -l" -o bin/heimdall
	docker build --target debug -t heimdall .
