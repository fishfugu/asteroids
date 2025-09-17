.PHONY: ecscan
ecscan:
	go build -o bin/ecscan ./cmd/ecscan

# optional tidy-up for bench if you move it
.PHONY: bench
bench:
	go build -o bin/bench ./cmd/bench