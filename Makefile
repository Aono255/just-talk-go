.PHONY: build install test clean run deps release-check

APP_NAME := just-talk
CMD_DIR := ./cmd/just-talk
BUILD_DIR := ./build

# Build for current platform
build:
	go build -o $(BUILD_DIR)/$(APP_NAME) $(CMD_DIR)

# Install to ~/.local/bin
install: build
	$(BUILD_DIR)/$(APP_NAME) --install

# Run (current platform)
run:
	go run $(CMD_DIR)

# Test
test:
	go test ./... -v

# Clean
clean:
	rm -rf $(BUILD_DIR)

# Install dependencies
deps:
	go mod tidy
	go mod download

# Local equivalent of the release workflow's per-platform build step.
# Mirrors .github/workflows/release.yml: one native archive for the current
# platform via GoReleaser, plus a formula render smoke check. No secrets needed.
RELEASE_GOOS := $(shell go env GOOS)
RELEASE_GOARCH := $(shell go env GOARCH)
# CGO_ENABLED must be set explicitly: .goreleaser.yaml templates it and errors
# when the variable is missing. Native cgo everywhere except Windows.
RELEASE_CGO := $(if $(filter windows,$(RELEASE_GOOS)),0,1)

release-check:
	goreleaser check
	goreleaser check .goreleaser.release.yaml
	RELEASE_TARGET=$(RELEASE_GOOS)-$(RELEASE_GOARCH) \
		CGO_ENABLED=$(RELEASE_CGO) \
		goreleaser release --clean --snapshot --skip=publish,validate
	@printf 'deadbeef  just-talk_darwin_arm64.tar.gz\ncafef00d  just-talk_darwin_amd64.tar.gz\n11112222  just-talk_linux_arm64.tar.gz\n33334444  just-talk_linux_amd64.tar.gz\n' > /tmp/just-talk-SHA256SUMS.txt
	TAG=v0.0.0 REPO=wakaka6/just-talk-go \
		SUMS_FILE=/tmp/just-talk-SHA256SUMS.txt \
		OUTPUT=/tmp/just-talk-formula.rb \
		./scripts/update-homebrew-formula.sh
	@rm -f /tmp/just-talk-SHA256SUMS.txt /tmp/just-talk-formula.rb
