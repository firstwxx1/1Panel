GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOARCH=$(shell go env GOARCH)
GOOS=$(shell go env GOOS)
VERSION ?= v2.4.1

BASE_PATH := $(shell pwd)
BUILD_PATH = $(BASE_PATH)/build
WEB_PATH=$(BASE_PATH)/frontend
ASSERT_PATH=$(BASE_PATH)/core/cmd/server/web/assets

CORE_PATH=$(BASE_PATH)/core
CORE_MAIN=$(CORE_PATH)/cmd/server/main.go
CORE_CONF=$(CORE_PATH)/cmd/server/conf/app.yaml
CORE_NAME=1panel-core

AGENT_PATH=$(BASE_PATH)/agent
AGENT_MAIN=$(AGENT_PATH)/cmd/server/main.go
AGENT_CONF=$(AGENT_PATH)/cmd/server/conf/app.yaml
AGENT_NAME=1panel-agent

define build_with_patched_conf
	@bak="$$(mktemp)"; cp "$(1)" "$$bak"; \
	trap 'cp "$$bak" "$(1)" 2>/dev/null || true; rm -f "$$bak"' EXIT; \
	hv=0; grep -q '^[[:space:]]*version:' "$(1)" && hv=1 || true; \
	awk -v ver="$(VERSION)" -v hv="$$hv" '\
		/^[[:space:]]*mode:/ { print "  mode: stable"; if (hv == 0) print "  version: " ver; next } \
		/^[[:space:]]*version:/ { print "  version: " ver; next } \
		/^[[:space:]]*level:/ { print "  level: info"; next } \
		{ print }' "$(1)" > "$(1).tmp" && cat "$(1).tmp" > "$(1)" && rm -f "$(1).tmp"; \
	mkdir -p "$(BUILD_PATH)" && cd "$(2)" && $(3)
endef

clean_assets:
	rm -rf $(ASSERT_PATH)

upx_bin:
	upx $(BUILD_PATH)/$(CORE_NAME)
	upx $(BUILD_PATH)/$(AGENT_NAME)

build_frontend: clean_assets
	cd $(WEB_PATH) && npm install && npm run build:pro

build_core_on_linux:
	$(call build_with_patched_conf,$(CORE_CONF),$(CORE_PATH),CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) $(GOBUILD) -trimpath -tags xpack -ldflags '-s -w' -o $(BUILD_PATH)/$(CORE_NAME) $(CORE_MAIN))

build_agent_on_linux:
	$(call build_with_patched_conf,$(AGENT_CONF),$(AGENT_PATH),CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) $(GOBUILD) -trimpath -tags xpack -ldflags '-s -w' -o $(BUILD_PATH)/$(AGENT_NAME) $(AGENT_MAIN))

build_core_on_darwin:
	$(call build_with_patched_conf,$(CORE_CONF),$(CORE_PATH),CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -trimpath -tags xpack -ldflags '-s -w' -o $(BUILD_PATH)/$(CORE_NAME) $(CORE_MAIN))

build_agent_on_darwin:
	$(call build_with_patched_conf,$(AGENT_CONF),$(AGENT_PATH),CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -trimpath -tags xpack -ldflags '-s -w' -o $(BUILD_PATH)/$(AGENT_NAME) $(AGENT_MAIN))

build_all: clean_assets build_frontend build_core_on_linux build_agent_on_linux

build_on_local: clean_assets build_frontend build_core_on_darwin build_agent_on_darwin
