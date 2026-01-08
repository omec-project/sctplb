# Copyright 2022-present Open Networking Foundation
# Copyright 2023-present Intel Corporation
#
# SPDX-License-Identifier: Apache-2.0
#

FROM golang:1.25.5-bookworm@sha256:2c7c65601b020ee79db4c1a32ebee0bf3d6b298969ec683e24fcbea29305f10e AS builder

WORKDIR $GOPATH/src/sctplb
COPY . .
RUN CGO_ENABLED=0 go install

FROM alpine:3.23@sha256:865b95f46d98cf867a156fe4a135ad3fe50d2056aa3f25ed31662dff6da4eb62 AS sctplb

LABEL maintainer="Aether SD-Core <dev@lists.aetherproject.org>" \
    description="ONF open source 5G Core Network" \
    version="Stage 3"

ARG DEBUG_TOOLS

# Install debug tools ~ 50MB (if DEBUG_TOOLS is set to true)
RUN if [ "$DEBUG_TOOLS" = "true" ]; then \
        apk update && apk add --no-cache -U vim strace net-tools curl netcat-openbsd bind-tools bash; \
        fi

COPY --from=builder /go/bin/* /usr/local/bin/.
