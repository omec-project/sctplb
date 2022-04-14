# Copyright 2019-present Open Networking Foundation
#
# SPDX-License-Identifier: Apache-2.0
#

FROM golang:1.17.5 AS lb 

LABEL maintainer="ONF <omec-dev@opennetworking.org>"

RUN apt-get update
RUN apt-get -y install vim 
RUN cd $GOPATH/src && mkdir -p sctplb
COPY . $GOPATH/src/sctplb
RUN cd $GOPATH/src/sctplb && go mod tidy && go install 

FROM lb AS sctplb
WORKDIR /sdcore
RUN mkdir -p /sdcore/bin
COPY --from=lb $GOPATH/bin/* /sdcore/bin/
WORKDIR /sdcore
