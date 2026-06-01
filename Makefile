BINARY := hv

.PHONY: build web go-build test test-web clean

# Full build: frontend first (populates internal/web/dist), then the embedded binary.
build: web go-build

# Build the Lit frontend into internal/web/dist, then restore the tracked
# .gitkeep that Vite's emptyOutDir removes (keeps `git status` clean and lets a
# fresh checkout `go build` before npm runs).
web:
	cd web && npm ci && npm run build
	touch internal/web/dist/.gitkeep

go-build:
	go build -o $(BINARY) ./cmd/hv

# Go tests only (fast; no Node required).
test:
	go test ./...

# Frontend unit tests.
test-web:
	cd web && npm ci && npm run test

clean:
	rm -f $(BINARY)
	rm -rf internal/web/dist
	mkdir -p internal/web/dist
	touch internal/web/dist/.gitkeep
