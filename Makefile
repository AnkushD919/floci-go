.PHONY: build run test clean docker-build

BINARY_NAME=floci
BUILD_DIR=bin

build:
	@if not exist $(BUILD_DIR) mkdir $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/floci

run: build
	./$(BUILD_DIR)/$(BINARY_NAME)

test:
	go test -v ./...

clean:
	@if exist $(BUILD_DIR) rmdir /s /q $(BUILD_DIR)

VERSION=0.0.1

docker-build:
	docker build -t ankush919/floci-go:latest -t ankush919/floci-go:v$(VERSION) .

docker-push: docker-build
	docker --config ./.docker push ankush919/floci-go:latest
	docker --config ./.docker push ankush919/floci-go:v$(VERSION)
