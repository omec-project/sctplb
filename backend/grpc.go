package backend

import (
	ctxt "context"
	"fmt"
	"os"

	"github.com/omec-project/sctplb/context"
	"github.com/omec-project/sctplb/logger"
	gClient "github.com/omec-project/sctplb/sdcoreAmfServer"
	"google.golang.org/grpc"
)

type backendNF struct {
	address string
	conn    *grpc.ClientConn
	gc      gClient.NgapServiceClient
	state   bool
	stream  gClient.NgapService_HandleMessageClient
}

func (b *backendNF) connectToServer(port int) {
	target := fmt.Sprintf("%s:%d", b.address, port)

	fmt.Println("Connecting to target ", target)

	var err error
	b.conn, err = grpc.Dial(target, grpc.WithInsecure())

	if err != nil {
		fmt.Println("did not connect: ", err)
		b.deleteBackendNF()
		return
	}

	b.gc = gClient.NewNgapServiceClient(b.conn)

	stream, err := b.gc.HandleMessage(ctxt.Background())
	if err != nil {
		logger.AppLog.Println("openn stream error ", err)
		b.deleteBackendNF()
		return
	}

	b.stream = stream
	b.state = true
	for {
		//INIT message to new NF instance
		context.Sctplb_Self().RanPool.Range(func(key, value interface{}) bool {
			req := gClient.SctplbMessage{}
			req.VerboseMsg = "Hello From SCTP LB !"
			req.Msgtype = gClient.MsgType_INIT_MSG
			req.SctplbId = os.Getenv("HOSTNAME")
			candidate := value.(*context.Ran)
			if candidate.RanId != nil {
				req.GnbId = *candidate.RanId
			} else {
				logger.AppLog.Printf("ran connection %v is exist without GnbId, so not sending this ran details to NF\n",
					candidate.GnbIp)
			}
			if err := stream.Send(&req); err != nil {
				logger.AppLog.Println("can not send: ", err)
			}
			logger.AppLog.Println("Send Request message")
			response, err := stream.Recv()
			if err != nil {
				logger.AppLog.Println("Response from server: error ", err)
				b.state = false
			} else {
				logger.AppLog.Printf("Init Response from Server %s server: %s\n", response.AmfId, response.VerboseMsg)
				b.state = true
			}
			return true
		})
		break
	}
	if b.state == true {
		go b.connectionOnState()
		go b.readFromServer()
	}
}

func (b *backendNF) readFromServer() {
	for {
		response, err := b.stream.Recv()
		if err != nil {
			logger.GrpcLog.Printf("Error in Recv %v, Stop listening for this server %v ", err, b.address)
			b.deleteBackendNF()
			return
		} else {
			if response.Msgtype == gClient.MsgType_INIT_MSG {
				logger.GrpcLog.Printf("Init Response from Server %s server: %s", response.AmfId, response.VerboseMsg)
			} else if response.Msgtype == gClient.MsgType_REDIRECT_MSG {
				var found bool
				for _, b1 := range backends {
					if b1.address == response.RedirectId {
						if b1.state == false {
							logger.GrpcLog.Printf("backend state is not in READY state, so not forwarding redirected Msg")
						} else {
							t := gClient.SctplbMessage{}
							t.VerboseMsg = "Hello From gNB Message !"
							t.Msgtype = gClient.MsgType_GNB_MSG
							t.SctplbId = os.Getenv("HOSTNAME")
							t.Msg = response.Msg
							t.GnbId = response.GnbId
							b1.stream.Send(&t)
							logger.GrpcLog.Printf("successfully forwarded msg to correct AMF")
							found = true
						}
						break
					}
				}
				if !found {
					logger.GrpcLog.Printf("dropping redirected message as backend ip [%v] is not exist", response.RedirectId)
				}

			} else {
				var ran *context.Ran
				//fetch ran connection based on GnbId
				if response.GnbId == "" {
					logger.RanLog.Printf("Received null GnbId from backend NF")
				} else if response.GnbIpAddr != "" {
					// GnbId may present NGSetupreponse/failure receives from NF
					ran, _ = context.Sctplb_Self().RanFindByGnbIp(response.GnbIpAddr)
					if ran != nil && response.GnbId != "" {
						ran.SetRanId(response.GnbId)
						logger.RanLog.Printf("Received GnbId: %v for GNbIpAddress: %v from NF", response.GnbId, response.GnbIpAddr)
					}
				} else if response.GnbId != "" {
					ran, _ = context.Sctplb_Self().RanFindByGnbId(response.GnbId)
				}
				if ran != nil {
					ran.Conn.Write(response.Msg)
				} else {
					logger.RanLog.Printf("Couldn't fetch sctp connection with GnbId: %v", response.GnbId)
				}
			}
		}
	}
}
