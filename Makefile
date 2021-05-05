TAG?=latest
VERSION?=$(shell grep 'VERSION' pkg/version/version.go | awk '{ print $$4 }' | tr -d '"')

build:
	CGO_ENABLED=0 go build -a -o ./bin/example ./cmd/example

fmt:
	gofmt -l -s -w ./
	goimports -l -w ./

test-fmt:
	gofmt -l -s ./ | grep ".*\.go"; if [ "$$?" = "0" ]; then exit 1; fi
	goimports -l ./ | grep ".*\.go"; if [ "$$?" = "0" ]; then exit 1; fi

codegen:
	./hack/update-codegen.sh

test-codegen:
	./hack/verify-codegen.sh
