BINARY := clickvault
BIN_DIR := bin

.PHONY: build
build:
	go build -o $(BIN_DIR)/$(BINARY) .

.PHONY: build-linux-amd64
build-linux-amd64:
	GOOS=linux GOARCH=amd64 go build -o $(BIN_DIR)/$(BINARY)-linux-amd64 .

.PHONY: build-linux-arm64
build-linux-arm64:
	GOOS=linux GOARCH=arm64 go build -o $(BIN_DIR)/$(BINARY)-linux-arm64 .

.PHONY: sha256
sha256: build-linux-amd64
	sha256sum $(BIN_DIR)/$(BINARY)-linux-amd64

.PHONY: clean
clean:
	rm -rf $(BIN_DIR)

.PHONY: test
test:
	go test ./...
