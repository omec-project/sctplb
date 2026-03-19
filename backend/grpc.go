// SPDX-FileCopyrightText: 2023 Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

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
	"google.golang.org/grpc/credentials/insecure"
)

const verboseGnbMsg = "Hello From gNB Message !"

func (b *GrpcServer) ConnectToServer(port int) {
	target := fmt.Sprintf("%s:%d", b.address, port)

	logger.AppLog.Infoln("connecting to target", target)

	var err error
	b.conn, err = grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		logger.AppLog.Errorln("did not connect:", err)
		deleteBackendNF(b)
		return
	}

	b.gc = gClient.NewNgapServiceClient(b.conn)

	stream, err := b.gc.HandleMessage(ctxt.Background())
	if err != nil {
		logger.AppLog.Errorw("open stream error", err)
		deleteBackendNF(b)
		return
	}

	b.stream = stream
	b.state = true
	for {
		// INIT message to new NF instance
		context.Sctplb_Self().RanPool.Range(func(key, value any) bool {
			req := gClient.SctplbMessage{}
			req.VerboseMsg = "Hello From SCTP LB!"
			req.Msgtype = gClient.MsgType_INIT_MSG
			req.SctplbId = os.Getenv("HOSTNAME")
			candidate := value.(*context.Ran)
			if candidate.N3iwfId != nil {
				req.N3IwfId = *candidate.N3iwfId
			} else if candidate.RanId != nil {
				req.GnbId = *candidate.RanId
			} else {
				logger.AppLog.Infof("ran connection %v exists without RanId or N3iwfId, so not sending this ran details to NF",
					candidate.GnbIp)
			}
			if err := stream.Send(&req); err != nil {
				logger.AppLog.Warnln("can not send:", err)
			}
			logger.AppLog.Infoln("send Request message")
			response, err := stream.Recv()
			if err != nil {
				logger.AppLog.Errorln("response from server: error", err)
				b.state = false
			} else {
				logger.AppLog.Infof("init Response from Server %s server: %s", response.AmfId, response.VerboseMsg)
				b.state = true
			}
			return true
		})
		break
	}
	if b.state {
		go b.connectionOnState()
		go b.readFromServer()
	}
}

func (b *GrpcServer) readFromServer() {
	for {
		response, err := b.stream.Recv()
		if err != nil {
			logger.GrpcLog.Errorf("error in Recv %v, Stop listening for this server %v", err, b.address)
			deleteBackendNF(b)
			return
		} else {
			if response.Msgtype == gClient.MsgType_INIT_MSG {
				logger.GrpcLog.Infof("init Response from Server %s server: %s", response.AmfId, response.VerboseMsg)
			} else if response.Msgtype == gClient.MsgType_REDIRECT_MSG {
				var found bool
				ctx := context.Sctplb_Self()
				for _, instance := range ctx.Backends {
					b1 := instance.(*GrpcServer)
					if b1.address == response.RedirectId {
						if !b1.state {
							logger.GrpcLog.Infoln("backend state is not in READY state, so not forwarding redirected Msg")
						} else {
							t := gClient.SctplbMessage{}
							t.SctplbId = os.Getenv("HOSTNAME")
							t.Msg = response.Msg
							if response.N3IwfId != "" {
								t.VerboseMsg = "redirected from AMF to N3IWF"
								t.Msgtype = gClient.MsgType_N3IWF_MSG
								t.N3IwfId = response.N3IwfId
							} else {
								t.VerboseMsg = verboseGnbMsg
								t.Msgtype = gClient.MsgType_GNB_MSG
								t.GnbId = response.GnbId
							}
							err := b1.stream.Send(&t)
							if err != nil {
								logger.GrpcLog.Infoln("error forwarding msg")
							}
							logger.GrpcLog.Infoln("successfully forwarded msg to correct AMF")
							found = true
						}
						break
					}
				}
				if !found {
					logger.GrpcLog.Infof("dropping redirected message as backend ip [%v] is not exist", response.RedirectId)
				}
			} else {
				var ran *context.Ran
				// fetch ran connection based on GnbId or N3iwfId
				switch {
				case response.GnbIpAddr != "" && response.GnbId != "":
					// NGSetupResponse for gNB: bind GnbId to the IP-keyed Ran
					ran, _ = context.Sctplb_Self().RanFindByGnbIp(response.GnbIpAddr)
					if ran != nil {
						ran.SetRanId(response.GnbId)
						logger.RanLog.Infof("received GnbId: %v for GnbIpAddress: %v from NF", response.GnbId, response.GnbIpAddr)
					}
				case response.GnbIpAddr != "" && response.N3IwfId != "":
					// NGSetupResponse for N3IWF: bind N3iwfId to the IP-keyed Ran
					ran, _ = context.Sctplb_Self().RanFindByGnbIp(response.GnbIpAddr)
					if ran != nil {
						ran.SetN3iwfId(response.N3IwfId)
						logger.RanLog.Infof("received N3iwfId: %v for GnbIpAddress: %v from NF", response.N3IwfId, response.GnbIpAddr)
					}
				case response.GnbId != "":
					ran, _ = context.Sctplb_Self().RanFindByGnbId(response.GnbId)
				case response.N3IwfId != "":
					ran, _ = context.Sctplb_Self().RanFindByN3iwfId(response.N3IwfId)
				default:
					logger.RanLog.Infoln("received message with no RAN identifier from backend NF")
				}
				if ran != nil {
					_, err := ran.Conn.Write(response.Msg)
					if err != nil {
						logger.RanLog.Infof("err %+v", err)
					}
				} else {
					logger.RanLog.Infof("couldn't fetch sctp connection with GnbId: %v or N3iwfId: %v", response.GnbId, response.N3IwfId)
				}
			}
		}
	}
}

func (b *GrpcServer) connectionOnState() {
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

func (b *GrpcServer) Send(msg []byte, end bool, ran *context.Ran) error {
	t := gClient.SctplbMessage{}
	t.SctplbId = os.Getenv("HOSTNAME")
	t.Msg = msg
	if end {
		if ran != nil && ran.N3iwfId != nil {
			t.VerboseMsg = "Bye From N3IWF Message !"
			t.Msgtype = gClient.MsgType_N3IWF_DISC
			t.N3IwfId = *ran.N3iwfId
		} else {
			t.VerboseMsg = "Bye From gNB Message !"
			t.Msgtype = gClient.MsgType_GNB_DISC
			if ran != nil && ran.RanId != nil {
				t.GnbId = *ran.RanId
			}
		}
	} else {
		if ran.N3iwfId != nil {
			t.VerboseMsg = "Hello From N3IWF Message !"
			t.Msgtype = gClient.MsgType_N3IWF_MSG
			t.N3IwfId = *ran.N3iwfId
		} else if ran.RanId != nil {
			t.VerboseMsg = verboseGnbMsg
			t.Msgtype = gClient.MsgType_GNB_MSG
			t.GnbId = *ran.RanId
		} else {
			// no ID yet (pre-NGSetup); send IP so AMF can bind the ID
			t.VerboseMsg = verboseGnbMsg
			t.Msgtype = gClient.MsgType_GNB_MSG
			t.GnbIpAddr = ran.Conn.RemoteAddr().String()
		}
	}
	return b.stream.Send(&t)
}

func (b *GrpcServer) State() bool {
	return b.state
}
