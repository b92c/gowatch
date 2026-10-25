install:
	make install-dependencies
	make install-hooks
install-dependencies:
	$(MAKE) install-tools
install-hooks:
	cp .setup/build/config/pre-commit .git/hooks &&\
	chmod +x .git/hooks/pre-commit
docker-build:
	docker compose build
build:
	go build -o bin/gowatch ./cmd/gowatch
run:
	./bin/gowatch
clean:
	rm -rf bin/
go-sec:
	gosec ./...
field-fix:
	fieldalignment -fix ./...

## TEST

#COVER_DIRS=./internal/usecases/...
COVER_DIRS=./...
test:
	make test-command
	go tool cover -func=coverage.out
test-coverage:
	make test-command
	go tool cover -html=coverage.out
test-command:
	go test -race -coverpkg=$(COVER_DIRS) -v -coverprofile=coverage.out $(COVER_DIRS)

## CI CHECKS
# `make check` runs the same lint steps as the CI "Lint" job, then the tests.
# `make install` and the CI install their tools through install-tools and
# install-govulncheck, so the versions below are the only place to bump them.

GOSEC_VERSION ?= v2.30.0
FIELDALIGNMENT_VERSION ?= v0.52.0
GOVULNCHECK_VERSION ?= v1.9.0

.PHONY: install-tools install-govulncheck fmt-check vet mod-check field-check lint vuln check

install-tools:
	go install github.com/securego/gosec/v2/cmd/gosec@$(GOSEC_VERSION)
	go install golang.org/x/tools/go/analysis/passes/fieldalignment/cmd/fieldalignment@$(FIELDALIGNMENT_VERSION)
install-govulncheck:
	go install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)
fmt-check:
	@files="$$(gofmt -l .)" || exit 1; \
	if [ -n "$$files" ]; then \
		echo "These files are not gofmt-formatted (run 'gofmt -w .'):"; \
		echo "$$files"; \
		exit 1; \
	fi
vet:
	go vet ./...
mod-check:
	go mod tidy -diff
field-check:
	@out="$$(fieldalignment ./... 2>&1)"; status=$$?; \
	if [ $$status -ne 0 ] || [ -n "$$out" ]; then \
		echo "$$out"; \
		echo "fieldalignment reported issues (see 'make field-fix')."; \
		exit 1; \
	fi
lint: fmt-check vet mod-check field-check go-sec
vuln:
	govulncheck ./...
check: lint test
