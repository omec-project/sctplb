package backend

import (
	"net"

	"github.com/omec-project/sctplb/config"
	"github.com/omec-project/sctplb/context"
	gClient "github.com/omec-project/sctplb/sdcoreAmfServer"
	"google.golang.org/grpc"
)

type SctpConnections struct {
	conn    net.Conn
	address string
}

type BackendSvc struct {
	Cfg config.Config
}

// SD-CORE AMF: use grpc protocol to receive ngap/nas message
var _ context.NF = &GrpcServer{}

type GrpcServer struct {
	address string
	conn    *grpc.ClientConn
	gc      gClient.NgapServiceClient
	state   bool
	stream  gClient.NgapService_HandleMessageClient
}
