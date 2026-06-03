BINARY := hv

.PHONY: build web web-deps web-build web-test go-build fmt fmt-check vet test test-web \
        ci ci-go ci-web clean

# Full build: frontend first (populates internal/web/dist), then the embedded binary.
build: web go-build

# Install web dependencies. Split out so CI (and the build/test targets) share a
# single reproducible install instead of re-running `npm ci` per step.
web-deps:
	cd web && npm ci

# Build the Lit frontend into internal/web/dist (assumes deps installed), then
# restore the tracked .gitkeep that Vite's emptyOutDir removes (keeps `git status`
# clean and lets a fresh checkout `go build` before npm runs).
web-build:
	cd web && npm run build
	touch internal/web/dist/.gitkeep

# Frontend unit tests (assumes deps installed).
web-test:
	cd web && npm run test

# Convenience: full frontend build / test from a clean checkout (deps + step).
web: web-deps web-build
test-web: web-deps web-test

go-build:
	go build -o $(BINARY) ./cmd/hv

# Format all tracked Go files in place.
fmt:
	git ls-files '*.go' | xargs gofmt -w

# Fail if any tracked Go file is not gofmt-clean (the CI formatting gate).
fmt-check:
	@unformatted=$$(git ls-files '*.go' | xargs gofmt -l); \
	if [ -n "$$unformatted" ]; then \
		echo "These files are not gofmt-clean:"; echo "$$unformatted"; \
		echo "Run: make fmt"; exit 1; \
	fi

vet:
	go vet ./...

# Go tests only (fast; no Node required).
test:
	go test ./...

# CI aggregates — single source of truth shared with .github/workflows/ci.yml.
# `make ci` reproduces the full pipeline locally.
ci-go: fmt-check vet test go-build
ci-web: web-deps web-build web-test
ci: ci-go ci-web

clean:
	rm -f $(BINARY)
	rm -rf internal/web/dist
	mkdir -p internal/web/dist
	touch internal/web/dist/.gitkeep
