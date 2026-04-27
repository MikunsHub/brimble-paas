.PHONY: test cover cover-func

TEST_PKGS := $(shell go list -f '{{if or .TestGoFiles .XTestGoFiles}}{{.ImportPath}}{{end}}' ./... | sed '/^$$/d')

test:
	go test ./... -v

cover:
	go test $(TEST_PKGS) -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	@echo "Open coverage.html to view report"

cover-func:
	go test $(TEST_PKGS) -coverprofile=coverage.out
	go tool cover -func=coverage.out
