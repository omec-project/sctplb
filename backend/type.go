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

var _ context.NF = &BackendNF{}

type BackendNF struct {
	address string
	conn    *grpc.ClientConn
	gc      gClient.NgapServiceClient
	state   bool
	stream  gClient.NgapService_HandleMessageClient
}
