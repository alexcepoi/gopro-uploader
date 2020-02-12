BINARY=gopro-uploader

.PHONY: all
all: build

.PHONY: build
build:
	go build -v -o bin/$(BINARY)

.PHONY: test
test:
	go test -v ./...

.PHONY: clean
clean:
	go clean
	rm -f bin/$(BINARY)
