
.PHONY: all build clean

BINARY_NAME=dogclaw

all: build

build:
	go build -o $(BINARY_NAME) ./cmd/dogclaw

clean:
	rm -f $(BINARY_NAME)
