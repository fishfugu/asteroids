# ---- config ----
BIN_DIR := bin
ECSCAN  := $(BIN_DIR)/ecscan
ECTORUS := $(BIN_DIR)/ectorus
BENCH   := $(BIN_DIR)/bench

GOFLAGS :=
LDFLAGS :=
PKGS := ./...

# ---- helpers ----
.PHONY: help
help:
	@echo "Targets:"
	@echo "  ecscan    - build ecscan CLI"
	@echo "  ectorus   - build ectorus tool"
	@echo "  bench     - build bench harness"
	@echo "  build     - build all binaries"
	@echo "  test      - run unit tests"
	@echo "  tidy      - go mod tidy"
	@echo "  clean     - remove bin/"

# ---- builds ----
.PHONY: ecscan
ecscan:
	@mkdir -p $(BIN_DIR)
	go build $(GOFLAGS) -o $(ECSCAN) ./cmd/ecscan

.PHONY: ectorus
ectorus:
	@mkdir -p $(BIN_DIR)
	go build $(GOFLAGS) -o $(ECTORUS) ./ectorus

.PHONY: build
build: ecscan ectorus bench

# ---- dev hygiene ----
.PHONY: test
test:
	go test -v $(PKGS)

.PHONY: tidy
tidy:
	go mod tidy

.PHONY: clean
clean:
	rm -rf $(BIN_DIR)

.PHONY: install-ecscan
install-ecscan:
	go install ./cmd/ecscan


# ---- benchmarking ----

.PHONY: bench-build benchscan-build bench-run benchscan-run

bench-build:
	@mkdir -p bin
	go build -o bin/bench ./cmd/bench

benchscan-build:
	@mkdir -p bin
	go build -o bin/benchscan ./cmd/benchscan

# make bench-run BENCH_ARGS='-args "-A 2 -B 3 -p 101 -json" -reps 3 -timeout 2m'
bench-run: bench-build
	@test -x bin/ectorus || (echo "Building ectorus..." && $(MAKE) ectorus)
	./bin/bench -ectorus ./bin/ectorus $(BENCH_ARGS)

# make benchscan-run BENCH_ARGS='-p 101 -A 2 -B 3 -runs 3 -warmup 1'
benchscan-run: benchscan-build
	@test -x bin/ecscan  || (echo "Building ecscan..." && $(MAKE) ecscan)
	./bin/benchscan -ecscan ./bin/ecscan $(BENCH_ARGS)