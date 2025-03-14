#
# Licensed to the Apache Software Foundation (ASF) under one or more
# contributor license agreements.  See the NOTICE file distributed with
# this work for additional information regarding copyright ownership.
# The ASF licenses this file to You under the Apache License, Version 2.0
# (the "License"); you may not use this file except in compliance with
# the License.  You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#

FIRST_GOPATH              := $(firstword $(subst :, ,$(GOPATH)))
PKGS                      := $(shell go list ./... | grep -v /tests | grep -v /xcpb | grep -v /gpb | grep -v /generated)
GOFILES_NOVENDOR          := $(shell find . -name vendor -prune -o -type f -name '*.go' -not -name '*.pb.go' -print)
GOFILES_BUILD             := $(shell find . -type f -name '*.go' -not -name '*_test.go')
PROTOFILES                := $(shell find . -name vendor -prune -o -type f -name '*.proto' -print)

ALLGOFILES				  			:= $(shell find . -type f -name '*.go' -not -name '*.pb.go')
DATE                      := $(shell date -u -d "@$(SOURCE_DATE_EPOCH)" '+%FT%T%z' 2>/dev/null || date -u '+%FT%T%z')

BUILDFLAGS_NOPIE		  :=
BUILDFLAGS                ?= $(BUILDFLAGS_NOPIE) -buildmode=pie
TESTFLAGS                 ?=
PWD                       := $(shell pwd)
PREFIX                    ?= $(GOPATH)
BINDIR                    ?= $(PREFIX)/bin
GO                        := go
GOOS                      ?= $(shell go version | cut -d' ' -f4 | cut -d'/' -f1)
GOARCH                    ?= $(shell go version | cut -d' ' -f4 | cut -d'/' -f2)
TAGS                      ?= netgo
SHELL = bash
GOFUMPT_SPLIT_LONG_LINES  := on

BINARIES := cmd/spark-connect-proxy/spark-connect-proxy

all: build

build: $(BINARIES)

cmd/spark-connect-proxy/spark-connect-proxy: $(GOFILES_BUILD)
	@echo ">> BUILD, output = $@"
	@cd $(dir $@) && $(GO) build -o $(notdir $@) $(BUILDFLAGS)
	@printf '%s\n' '$(OK)'


fmt:
	@echo -n ">> glongci-lint: fix"
	env GOFUMPT_SPLIT_LONG_LINES=$(GOFUMPT_SPLIT_LONG_LINES) golangci-lint run --fix


test: $(BUILD_OUTPUT)
	@echo ">> TEST, \"verbose\""
	@$(foreach pkg, $(PKGS),\
	    @echo -n "     ";\
		$(GO) test -v -run '(Test|Example)' $(BUILDFLAGS) $(TESTFLAGS) $(pkg) || exit 1)

coverage: $(BUILD_OUTPUT)
	@echo ">> TEST, \"coverage\""
	@$(GO) test -cover -coverprofile=coverage.out -covermode=atomic ./internal/...
	@$(GO) tool cover -html=coverage.out -o coverage.html

check:
	@echo -n ">> CHECK"
	./dev/check-license
	@echo -n ">> glongci-lint: "
	env GOFUMPT_SPLIT_LONG_LINES=$(GOFUMPT_SPLIT_LONG_LINES) golangci-lint run

serve-docs:
	@echo ">> DOCS: Installing godoc (if needed) and serving documentation at http://localhost:6060"
	@go install golang.org/x/tools/cmd/godoc@latest
	@echo ">> Starting godoc server. Press Ctrl+C to stop."
	@$(shell go env GOPATH)/bin/godoc -http=:6060