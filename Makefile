BINARY := terraform-provider-eighthundred
HOSTNAME := registry.terraform.io
NAMESPACE := ziinc
NAME := eighthundred
VERSION := 0.1.0
OS_ARCH := $(shell go env GOOS)_$(shell go env GOARCH)
INSTALL_DIR := $(HOME)/.terraform.d/plugins/$(HOSTNAME)/$(NAMESPACE)/$(NAME)/$(VERSION)/$(OS_ARCH)

.PHONY: build install test testacc lint docs clean fmt vet tidy

build:
	go build -o bin/$(BINARY) .

install: build
	mkdir -p $(INSTALL_DIR)
	cp bin/$(BINARY) $(INSTALL_DIR)/$(BINARY)_v$(VERSION)
	@echo "Installed to $(INSTALL_DIR)"

test:
	go test ./... -count=1

testacc:
	@if [ -z "$$EIGHT_HUNDRED_API_TOKEN" ]; then echo "EIGHT_HUNDRED_API_TOKEN must be set" >&2; exit 1; fi
	TF_ACC=1 go test ./internal/provider/... -v -count=1 -timeout 30m

lint:
	golangci-lint run ./...

docs:
	go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs generate

fmt:
	go fmt ./...
	terraform fmt -recursive examples/

vet:
	go vet ./...

tidy:
	go mod tidy

clean:
	rm -rf bin/ dist/
