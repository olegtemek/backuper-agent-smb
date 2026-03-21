.PHONY: build run test clean build-all build-windows build-linux build-macos

build:
	go build -o backuper-agent ./cmd/main

run:
	go run ./cmd/main/main.go

test:
	go test -v ./...

clean:
	rm -f backuper-agent
	rm -rf build/

build-all: build-windows build-linux build-macos

build-windows:
	mkdir -p build
	GOOS=windows GOARCH=amd64 go build -o build/backuper-agent-windows.exe ./cmd/main

build-linux:
	mkdir -p build
	GOOS=linux GOARCH=amd64 go build -o build/backuper-agent-linux ./cmd/main

build-macos:
	mkdir -p build
	GOOS=darwin GOARCH=amd64 go build -o build/backuper-agent-macos-intel ./cmd/main
	GOOS=darwin GOARCH=arm64 go build -o build/backuper-agent-macos-arm ./cmd/main
