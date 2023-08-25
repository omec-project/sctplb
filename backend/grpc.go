package backend

import (
	ctxt "context"
	"fmt"
	"os"

	"github.com/omec-project/sctplb/context"
	"github.com/omec-project/sctplb/logger"
	gClient "github.com/omec-project/sctplb/sdcoreAmfServer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
)

func (b *BackendNF) ConnectToServer(port int) {
	target := fmt.Sprintf("%s:%d", b.address, port)

	fmt.Println("Connecting to target ", target)

	var err error
	b.conn, err = grpc.Dial(target, grpc.WithInsecure())

	if err != nil {
		fmt.Println("did not connect: ", err)
		deleteBackendNF(b)
		return
	}

	b.gc = gClient.NewNgapServiceClient(b.conn)

	stream, err := b.gc.HandleMessage(ctxt.Background())
	if err != nil {
		logger.AppLog.Println("openn stream error ", err)
		deleteBackendNF(b)
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

func (b *BackendNF) readFromServer() {
	for {
		response, err := b.stream.Recv()
		if err != nil {
			logger.GrpcLog.Printf("Error in Recv %v, Stop listening for this server %v ", err, b.address)
			deleteBackendNF(b)
			return
		} else {
			if response.Msgtype == gClient.MsgType_INIT_MSG {
				logger.GrpcLog.Printf("Init Response from Server %s server: %s", response.AmfId, response.VerboseMsg)
			} else if response.Msgtype == gClient.MsgType_REDIRECT_MSG {
				var found bool
				ctx := context.Sctplb_Self()
				for _, instance := range ctx.Backends {
					b1 := instance.(*BackendNF)
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

func (b *BackendNF) connectionOnState() {

	go func() {

		// continue checking for state change
		// until one of break states is found
		for {
			change := b.conn.WaitForStateChange(ctxt.Background(), b.conn.GetState())
			if change && b.conn.GetState() == connectivity.Idle {
				deleteBackendNF(b)
				return
			}

		}
	}()

}

func (b *BackendNF) Send(msg []byte, end bool, ran *context.Ran) error {
	t := gClient.SctplbMessage{}
	if end {
		t.VerboseMsg = "Bye From gNB Message !"
		t.Msgtype = gClient.MsgType_GNB_DISC
		t.SctplbId = os.Getenv("HOSTNAME")
		if ran != nil && ran.RanId != nil {
			t.GnbId = *ran.RanId
		}
		t.Msg = msg
	} else {
		t.VerboseMsg = "Hello From gNB Message !"
		t.Msgtype = gClient.MsgType_GNB_MSG
		t.SctplbId = os.Getenv("HOSTNAME")
		//send GnbId to backendNF if exist
		//GnbIp to backend ig GnbId is not exist, mostly this is for NGSetup Message
		if ran.RanId != nil {
			t.GnbId = *ran.RanId
		} else {
			t.GnbIpAddr = ran.Conn.RemoteAddr().String()
		}
		t.Msg = msg
	}
	return b.stream.Send(&t)
}

func (b *BackendNF) State() bool {
	return b.state
}
