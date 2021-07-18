all: build

.PHONY: install-tools
install-tools:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.26.0
	go install golang.org/x/tools/cmd/stringer@v0.1.2


.PHONY: protoc
protoc:
	protoc --proto_path=. --go_out=. gdxsv/proto/*.proto
	cd ./flycast/core/gdxsv && protoc --proto_path=../../../gdxsv/proto --cpp_out=. gdxsv.proto


.PHONY: build
build:
	mkdir -p bin
	go generate ./gdxsv
	go build -ldflags "-X main.gdxsvVersion=$(shell git describe --tags --abbrev=0) -X main.gdxsvRevision=$(shell git rev-parse --short HEAD)" -o bin/gdxsv ./gdxsv


.PHONY: test
test:
	go test -race -v ./...


.PHONY: lint
lint:
	golangci-lint run


.PHONY: fmt
fmt:
	go fmt ./...


.PHONY: race
race:
	mkdir -p bin
	go generate ./gdxsv
	go build -race -ldflags "-X main.gdxsvVersion=$(shell git describe --tags --abbrev=0) -X main.gdxsvRevision=$(shell git rev-parse --short HEAD)" -o bin/gdxsv ./gdxsv

.PHONY: ci
ci:
	mkdir -p bin
	go generate ./gdxsv
	go build -ldflags "-X main.gdxsvVersion=$(shell git describe --tags --abbrev=0) -X main.gdxsvRevision=$(shell git rev-parse --short HEAD)" -o bin/gdxsv ./gdxsv
	go test -race -v -coverprofile=coverage.txt -covermode=atomic ./... 


.PHONY: release
release:
	mkdir -p bin
	go generate ./gdxsv
	go build -ldflags "-X main.gdxsvVersion=$(shell git describe --tags --abbrev=0) -X main.gdxsvRevision=$(shell git rev-parse --short HEAD)" -o bin/gdxsv ./gdxsv

