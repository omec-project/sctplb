// SPDX-FileCopyrightText: 2023 Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/ishidawataru/sctp"
	"github.com/omec-project/sctplb/config"
	sctplbCtx "github.com/omec-project/sctplb/context"
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
var _ sctplbCtx.NF = &GrpcServer{}

type GrpcServer struct {
	gc     gClient.NgapServiceClient
	stream gClient.NgapService_HandleMessageClient
	conn   *grpc.ClientConn
	// readDone is closed when readFromServer returns, so Close can wait for
	// the stream to drain instead of aborting it.
	readDone chan struct{}
	// streamCancel cancels the stream's own lifecycle context. It is kept
	// independent of the shutdown signal context so SIGTERM cannot abort the
	// stream before Close has a chance to drain it.
	streamCancel context.CancelFunc
	address      string
	// sendMu serializes every stream.Send/CloseSend call: gRPC permits only
	// one sender goroutine per stream at a time.
	sendMu sync.Mutex
	state  atomic.Bool
}
