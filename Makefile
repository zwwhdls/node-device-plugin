# Copyright 2023 Juicedata Inc
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

GO111MODULE=on
VERSION=$(shell git describe --tags --match 'v*' --always --dirty)
GIT_BRANCH?=$(shell git rev-parse --abbrev-ref HEAD)
GIT_COMMIT?=$(shell git rev-parse HEAD)
DEV_TAG=dev-$(shell git describe --always --dirty)
BUILD_DATE?=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
PKG=github.com/zwwhdls/node-device-plugin
LDFLAGS?="-X ${PKG}/plugin.driverVersion=${VERSION} -X ${PKG}/plugin.gitCommit=${GIT_COMMIT} -X ${PKG}/plugin.buildDate=${BUILD_DATE} -s -w"
IMAGE?=zwwhdls/node-device-plugin

# Build go binaries
.PHONY: build
build:
	mkdir -p bin
	CGO_ENABLED=0 GOOS=linux go build -ldflags ${LDFLAGS} -o bin/node-device-plugin ./cmd/

# Build docker image
.PHONY: image-dev
image-dev: build
	docker build -t $(IMAGE):$(DEV_TAG) .
	docker push $(IMAGE):$(DEV_TAG)

