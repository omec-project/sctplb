# Copyright 2022-present Open Networking Foundation
#
# SPDX-License-Identifier: Apache-2.0
#

PROJECT_NAME             := sctplb
VERSION                  ?= $(shell cat ./VERSION)

## Docker related
DOCKER_REGISTRY          ?=
DOCKER_REPOSITORY        ?=
DOCKER_TAG               ?= ${VERSION}
DOCKER_IMAGENAME         := ${DOCKER_REGISTRY}${DOCKER_REPOSITORY}${PROJECT_NAME}:${DOCKER_TAG}
DOCKER_BUILDKIT          ?= 1
DOCKER_BUILD_ARGS        ?=

## Docker labels. Only set ref and commit date if committed
#DOCKER_LABEL_VCS_URL     ?= $(shell git remote get-url $(shell git remote))
#DOCKER_LABEL_VCS_REF     ?= $(shell git diff-index --quiet HEAD -- && git rev-parse HEAD || echo "unknown")
#DOCKER_LABEL_COMMIT_DATE ?= $(shell git diff-index --quiet HEAD -- && git show -s --format=%cd --date=iso-strict HEAD || echo "unknown" )
DOCKER_LABEL_BUILD_DATE  ?= $(shell date -u "+%Y-%m-%dT%H:%M:%SZ")

DOCKER_TARGETS           ?= sctplb

.PHONY: docker-build docker-push

# https://docs.docker.com/engine/reference/commandline/build/#specifying-target-build-stage---target
docker-build:
	for target in $(DOCKER_TARGETS); do \
		DOCKER_BUILDKIT=$(DOCKER_BUILDKIT) docker build  $(DOCKER_BUILD_ARGS) \
			--target $$target \
			--tag ${DOCKER_REGISTRY}${DOCKER_REPOSITORY}$$target:${DOCKER_TAG} \
			--build-arg org_label_schema_version="${VERSION}" \
			--build-arg org_label_schema_vcs_url="${DOCKER_LABEL_VCS_URL}" \
			--build-arg org_label_schema_vcs_ref="${DOCKER_LABEL_VCS_REF}" \
			--build-arg org_label_schema_build_date="${DOCKER_LABEL_BUILD_DATE}" \
			--build-arg org_opencord_vcs_commit_date="${DOCKER_LABEL_COMMIT_DATE}" \
			. \
			|| exit 1; \
	done

docker-push:
	for target in $(DOCKER_TARGETS); do \
		docker push ${DOCKER_REGISTRY}${DOCKER_REPOSITORY}$$target:${DOCKER_TAG}; \
	done

.coverage:
	rm -rf $(CURDIR)/.coverage
	mkdir -p $(CURDIR)/.coverage

test: .coverage
	docker run --rm -v $(CURDIR):/smf -w /smf golang:latest \
		go test \
			-race \
			-failfast \
			-coverprofile=.coverage/coverage-unit.txt \
			-covermode=atomic \
			-v \
			./ ./...

fmt:
	@go fmt ./...

golint:
	@docker run --rm -v $(CURDIR):/app -w /app golangci/golangci-lint:latest golangci-lint run -v --config /app/.golangci.yml

check-reuse:
	@docker run --rm -v $(CURDIR):/smf -w /smf omecproject/reuse-verify:latest reuse lint
