# Copyright 2022-present Open Networking Foundation
# Copyright 2023-present Intel Corporation
#
# SPDX-License-Identifier: Apache-2.0
#

FROM golang:1.25.4-bookworm@sha256:e17419604b6d1f9bc245694425f0ec9b1b53685c80850900a376fb10cb0f70cb AS builder

WORKDIR $GOPATH/src/sctplb
COPY . .
RUN CGO_ENABLED=0 go install

FROM alpine:3.23@sha256:51183f2cfa6320055da30872f211093f9ff1d3cf06f39a0bdb212314c5dc7375 AS sctplb

LABEL maintainer="Aether SD-Core <dev@lists.aetherproject.org>" \
    description="ONF open source 5G Core Network" \
    version="Stage 3"

ARG DEBUG_TOOLS

# Install debug tools ~ 50MB (if DEBUG_TOOLS is set to true)
RUN if [ "$DEBUG_TOOLS" = "true" ]; then \
        apk update && apk add --no-cache -U vim strace net-tools curl netcat-openbsd bind-tools bash; \
        fi

COPY --from=builder /go/bin/* /usr/local/bin/.
