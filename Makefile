.PHONY: build test vet fmt lint golden clean

build:
	go build -o bin/sigma ./cmd/sigma

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -l -w .

# lint runs fmt-check + vet; fails if any file needs formatting.
lint:
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi
	go vet ./...

# golden regenerates golden fixtures. Scoped to packages that define -update.
golden:
	go test ./internal/agent/ -run Golden -update

clean:
	rm -rf bin
