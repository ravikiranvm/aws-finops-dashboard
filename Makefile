BINARY := finops
PKG    := ./...

.PHONY: help build test vet fmt verify golden run clean tidy

help:
	@echo "make build    compile ./$(BINARY)"
	@echo "make test     unit tests + golden frames"
	@echo "make golden   regenerate golden frames (review the diff!)"
	@echo "make verify   fmt check + vet + build + test   <- must pass"
	@echo "make run      build and run"
	@echo "make tidy     go mod tidy"

build:
	go build -o $(BINARY) ./cmd/finops

test:
	go test $(PKG)

vet:
	go vet $(PKG)

fmt:
	gofmt -w .

# Fails if anything is unformatted. gofmt is canonical, so formatting is never
# a review discussion.
fmt-check:
	@unformatted=$$(gofmt -l . 2>/dev/null); \
	if [ -n "$$unformatted" ]; then \
		echo "not gofmt'd:"; echo "$$unformatted"; exit 1; \
	fi

# Regenerate golden frames. Always read the diff before committing — a golden
# file updated without review defeats the entire point of having one.
golden:
	go test $(PKG) -update

verify: fmt-check vet build test
	@echo "verify: ok"

run: build
	./$(BINARY)

clean:
	rm -f $(BINARY)

tidy:
	go mod tidy
