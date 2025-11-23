PACKAGE_VERSION ?= 0.0.0

.PHONY: package-linux
package-linux:
	@echo "Packaging for Linux (VERSION=$(PACKAGE_VERSION))"
	VERSION=$(PACKAGE_VERSION) bash ./scripts/package_linux.sh

.PHONY: build
build:
	go build -o pancakeos
