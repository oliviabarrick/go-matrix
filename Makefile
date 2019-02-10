.PHONY: generate-client

pkg/api-docs.json:
	curl -Lo pkg/api-docs.json https://matrix.org/docs/api/client-server/json/api-docs.json

generate-client: pkg/api-docs.json
	docker run --rm -it -e GOPATH -v $(shell pwd)/pkg/:$(shell pwd)/pkg/ -w $(shell pwd)/pkg/ quay.io/goswagger/swagger:0.14.0 generate client -f api-docs.json
