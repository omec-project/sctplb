// SPDX-FileCopyrightText: 2023 Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"github.com/ishidawataru/sctp"
	"github.com/omec-project/sctplb/config"
	"github.com/omec-project/sctplb/context"
	gClient "github.com/omec-project/sctplb/sdcoreAmfServer"
	"google.golang.org/grpc"
)

type SctpConnections struct {
	conn    *sctp.SCTPConn
	address string
}

type BackendSvc struct {
	Cfg config.Config
}

// SD-CORE AMF: use grpc protocol to receive ngap/nas message
var _ context.NF = &GrpcServer{}

type GrpcServer struct {
	gc      gClient.NgapServiceClient
	stream  gClient.NgapService_HandleMessageClient
	conn    *grpc.ClientConn
	address string
	state   bool
}
