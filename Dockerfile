# Copyright 2022-present Open Networking Foundation
# Copyright 2023-present Intel Corporation
#
# SPDX-License-Identifier: Apache-2.0
#

FROM golang:1.23.0-bookworm AS builder

LABEL maintainer="Aether SD-Core <dev@lists.aetherproject.org>"

WORKDIR $GOPATH/src/sctplb
COPY . .
RUN CGO_ENABLED=0 go install

FROM alpine:3.20 AS sctplb

LABEL description="ONF open source 5G Core Network" \
    version="Stage 3"

ARG DEBUG_TOOLS

# Install debug tools ~ 50MB (if DEBUG_TOOLS is set to true)
RUN if [ "$DEBUG_TOOLS" = "true" ]; then \
        apk update && apk add --no-cache -U vim strace net-tools curl netcat-openbsd bind-tools bash; \
        fi

WORKDIR /sdcore
RUN mkdir -p bin/
COPY --from=builder /go/bin/* /sdcore/bin/
WORKDIR /sdcore
