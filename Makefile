.PHONY: generate-client

api-docs.json:
	curl -LO https://matrix.org/docs/api/client-server/json/api-docs.json

generate-client: api-docs.json
	docker run --rm -it -e GOPATH -v $(shell pwd):$(shell pwd) -w $(shell pwd) quay.io/goswagger/swagger:0.14.0 generate client -f api-docs.json

