all: build

.PHONY: build
build:
	mkdir -p bin
	go generate ./gdxsv
	go build -ldflags "-X main.gdxsvVersion=$(shell git describe --tags --abbrev=0) -X main.gdxsvRevision=$(shell git rev-parse --short HEAD)" -o bin/gdxsv ./gdxsv


.PHONY: test
test:
	go test -v ./...


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
	go test -race -v ./...

.PHONY: release
release:
	mkdir -p bin
	go generate ./gdxsv
	go build -ldflags "-X main.gdxsvVersion=$(shell git describe --tags --abbrev=0) -X main.gdxsvRevision=$(shell git rev-parse --short HEAD)" -o bin/gdxsv ./gdxsv

