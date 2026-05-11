# Copyright 2023-present Intel Corporation
# Copyright 2022-present Open Networking Foundation
#
# SPDX-License-Identifier: Apache-2.0
#

FROM golang:1.26.3-bookworm@sha256:252599aeb51ad60b83e4d8821802068127c528c707cb7dd7afd93be057c6011c AS builder

WORKDIR $GOPATH/src/sctplb
COPY . .
ARG MAKEFLAGS
RUN make all

FROM alpine:3.23@sha256:5b10f432ef3da1b8d4c7eb6c487f2f5a8f096bc91145e68878dd4a5019afde11 AS sctplb

# Build arguments for dynamic labels
ARG VERSION=dev
ARG VCS_URL=unknown
ARG VCS_REF=unknown
ARG BUILD_DATE=unknown

LABEL org.opencontainers.image.source="${VCS_URL}" \
    org.opencontainers.image.version="${VERSION}" \
    org.opencontainers.image.created="${BUILD_DATE}" \
    org.opencontainers.image.revision="${VCS_REF}" \
    org.opencontainers.image.url="${VCS_URL}" \
    org.opencontainers.image.title="sctplb" \
    org.opencontainers.image.description="Aether 5G Core SCTPLB Network Function" \
    org.opencontainers.image.authors="Aether SD-Core <dev@lists.aetherproject.org>" \
    org.opencontainers.image.vendor="Aether Project" \
    org.opencontainers.image.licenses="Apache-2.0" \
    org.opencontainers.image.documentation="https://docs.sd-core.aetherproject.org/"

ARG DEBUG_TOOLS

RUN if [ "$DEBUG_TOOLS" = "true" ]; then \
        apk add --no-cache vim nano strace net-tools curl netcat-openbsd bind-tools; \
    fi

# Copy executable
COPY --from=builder /go/src/sctplb/bin/* /usr/local/bin/.
