BIN     := logwiz
CMD     := ./cmd/logwiz
SIM_BIN := logsim
SIM_CMD := ./cmd/logsim

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

LDFLAGS := -ldflags "-X main.Version=$(VERSION)"

.PHONY: build build-sim test clean

build:
	go build $(LDFLAGS) -o $(BIN) $(CMD)

build-sim:
	go test ./cmd/logsim/...
	go build -o $(SIM_BIN) $(SIM_CMD)

test:
	go test ./...

clean:
	rm -f $(BIN) $(SIM_BIN)
