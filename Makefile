BIN     := logwiz
CMD     := ./cmd/logwiz
SIM_BIN := logsim
SIM_CMD := ./cmd/logsim

.PHONY: build build-sim test clean

build:
	go build -o $(BIN) $(CMD)

build-sim:
	go build -o $(SIM_BIN) $(SIM_CMD)

test:
	go test ./...

clean:
	rm -f $(BIN) $(SIM_BIN)
