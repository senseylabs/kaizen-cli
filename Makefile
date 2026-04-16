.PHONY: build install uninstall test lint clean dev

BINARY_NAME=kaizen
VERSION?=0.1.0

build:
	go build -ldflags "-X main.version=$(VERSION)" -o bin/$(BINARY_NAME) .

install: build
	sudo cp bin/$(BINARY_NAME) /usr/local/bin/$(BINARY_NAME)

uninstall:
	sudo rm -f /usr/local/bin/$(BINARY_NAME)

test:
	go test ./...

lint:
	golangci-lint run

clean:
	rm -rf bin/ dist/

dev:
	go build -ldflags "-X main.version=$(VERSION)-dev" -o bin/$(BINARY_NAME) .
