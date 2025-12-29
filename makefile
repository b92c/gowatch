install:
	make install-dependencies
	make install-hooks
install-dependencies:
	go install github.com/securego/gosec/v2/cmd/gosec@latest
	go install golang.org/x/tools/go/analysis/passes/fieldalignment/cmd/fieldalignment@latest
install-hooks:
	cp .setup/build/config/pre-commit .git/hooks &&\
	chmod +x .git/hooks/pre-commit
docker-build:
	docker compose build
build:
	go build -o bin/gowatch cmd/gowatch/main.go
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