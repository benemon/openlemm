GO_LICENSES_VERSION := v1.6.0
GO_LICENSES         := $(shell go env GOPATH)/bin/go-licenses

LICENCE_ALLOWLIST := MIT,Apache-2.0,BSD-2-Clause,BSD-3-Clause,ISC,MPL-2.0

.PHONY: help
help: ## List targets
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "%-16s %s\n", $$1, $$2}'

.PHONY: build
build: ## Build the openlemm binary
	go build -trimpath -o openlemm ./cmd/openlemm

.PHONY: check-fmt
check-fmt: ## Fail on Go files gofmt would change
	@out=$$(gofmt -l $$(git ls-files '*.go')); \
	if [ -n "$$out" ]; then echo "$$out"; echo "FAIL: gofmt"; exit 1; fi

.PHONY: vet
vet: ## Vet the module
	go vet ./...

.PHONY: test
test: ## Test the module; tests needing the disc or recordings skip without them
	go test ./...

$(GO_LICENSES):
	go install github.com/google/go-licenses@$(GO_LICENSES_VERSION)

.PHONY: check-licenses
# A failure means a dependency changed licence on upgrade: investigate the
# module, do not widen the list to get green. The module itself is
# GPL-3.0-or-later and is ignored; its dependencies are still checked.
check-licenses: $(GO_LICENSES) ## Fail on dependency licences outside the allowlist
	$(GO_LICENSES) check ./... --allowed_licenses=$(LICENCE_ALLOWLIST) --ignore github.com/benemon/openlemm

.PHONY: check-markers
# The patterns are assembled from fragments so this recipe never contains the
# strings it hunts and the Makefile is scanned like every other tracked file.
check-markers: ## Fail on AI-tooling markers in tracked files
	@p="cla""ude|anthro""pic|co""dex|open""ai|chat""gpt|gpt-[0-9o]|copi""lot|gem""ini|wind""surf|ai-gen""erated|co-auth""ored-by"; \
	if hits=$$(git grep -n -i -E "$$p" -- .); then \
		echo "$$hits"; echo "FAIL: AI-tooling markers in tracked files"; exit 1; \
	fi; \
	echo "no AI-tooling markers in tracked files"

.PHONY: check-layers
# The simulation and the disc and level readers are tested with go test alone;
# Ebitengine only draws, plays sound and reads input.
check-layers: ## Fail when internal/sim, internal/level or internal/disc depends on Ebitengine
	@for pkg in sim level disc; do \
		if go list -deps ./internal/$$pkg | grep -q '^github.com/hajimehoshi/ebiten'; then \
			echo "FAIL: internal/$$pkg depends on Ebitengine"; exit 1; \
		fi; \
	done; \
	echo "internal/sim, internal/level and internal/disc do not depend on Ebitengine"

.PHONY: check
check: check-fmt vet test check-licenses check-markers check-layers ## Run every CI gate
