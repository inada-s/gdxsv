all: build

# all build for local environment.
.PHONY: build
build:
	mkdir -p bin
	go generate ./gdxsv
	go build -o bin/gdxsv ./gdxsv

# ci
.PHONY: ci
ci:
	mkdir -p bin
	go generate ./gdxsv
	go build -o bin/gdxsv ./gdxsv
