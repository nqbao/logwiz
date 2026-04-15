BIN := logwiz
CMD := ./cmd/logwiz

.PHONY: build test clean

build:
	go build -o $(BIN) $(CMD)

test:
	go test ./...

clean:
	rm -f $(BIN)
