SHELL := /usr/bin/env bash

.PHONY: protobuf
protobuf:  ## Generate Go code from protocol buffers
	cd ../.. && \
	find ./api -type f -name '*.proto' -not -path "./api/google/*" -print0 | xargs -0 -n 1 -t -x -I {} protoc {} \
	  --go_out=./src/backend/pkg \
	  --go-grpc_out=./src/backend/pkg \
	  --go_opt=paths=source_relative \
	  --go-grpc_opt=paths=source_relative \
	  --grpc-gateway_out ./src/backend/pkg \
	  --grpc-gateway_opt paths=source_relative \
	  --proto_path=. \

.PHONY: lint
lint:  ## Run linter checks
	command -v golangci-lint > /dev/null 2>&1 || {\
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin $(VERSION_GOLANGCI_LINT);\
	}
	golangci-lint -v run