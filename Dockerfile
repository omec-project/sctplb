# Copyright 2022-present Open Networking Foundation
# Copyright 2023-present Intel Corporation
#
# SPDX-License-Identifier: Apache-2.0
#

FROM golang:1.26.0-bookworm@sha256:eae3cdfa040d0786510a5959d36a836978724d03b34a166ba2e0e198baac9196 AS builder

WORKDIR $GOPATH/src/sctplb
COPY . .
RUN CGO_ENABLED=0 go install

FROM alpine:3.23@sha256:25109184c71bdad752c8312a8623239686a9a2071e8825f20acb8f2198c3f659 AS sctplb

LABEL maintainer="Aether SD-Core <dev@lists.aetherproject.org>" \
    description="ONF open source 5G Core Network" \
    version="Stage 3"

ARG DEBUG_TOOLS

# Install debug tools ~ 50MB (if DEBUG_TOOLS is set to true)
RUN if [ "$DEBUG_TOOLS" = "true" ]; then \
        apk update && apk add --no-cache -U vim strace net-tools curl netcat-openbsd bind-tools bash; \
        fi

COPY --from=builder /go/bin/* /usr/local/bin/.
