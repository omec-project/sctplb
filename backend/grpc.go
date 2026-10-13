// SPDX-FileCopyrightText: 2023 Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"context"
	"fmt"
	"os"

	sctplbCtx "github.com/omec-project/sctplb/context"
	"github.com/omec-project/sctplb/logger"
	gClient "github.com/omec-project/sctplb/sdcoreAmfServer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
)

func (b *GrpcServer) ConnectToServer(ctx context.Context, port int) {
	if ctx.Err() != nil {
		deleteBackendNF(b)
		return
	}

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

	// The stream lives on its own context, not the shutdown signal ctx: SIGTERM
	// must not cancel in-flight RPCs before Close can drain them gracefully.
	streamCtx, cancel := context.WithCancel(context.Background())
	b.streamCancel = cancel
	stream, err := b.gc.HandleMessage(streamCtx)
	if err != nil {
		cancel()
		logger.AppLog.Errorw("open stream error", err)
		deleteBackendNF(b)
		return
	}

	b.stream = stream
	b.state.Store(true)

	// Until initialization finishes, ctx.Done() still aborts the stream, so a
	// SIGTERM during a stuck INIT handshake can't stall connectWg.Wait().
	initDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			cancel()
		case <-initDone:
		}
	}()

	for {
		// INIT message to new NF instance
		sctplbCtx.Sctplb_Self().RanPool.Range(func(key, value any) bool {
			req := gClient.SctplbMessage{}
			req.VerboseMsg = "Hello From SCTP LB!"
			req.Msgtype = gClient.MsgType_INIT_MSG
			req.SctplbId = os.Getenv("HOSTNAME")
			candidate := value.(*sctplbCtx.Ran)
			if candidate.RanId != nil {
				req.GnbId = *candidate.RanId
			} else {
				logger.AppLog.Infof("ran connection %v is exist without GnbId, so not sending this ran details to NF",
					candidate.GnbIp)
			}
			b.sendMu.Lock()
			err := stream.Send(&req)
			b.sendMu.Unlock()
			if err != nil {
				logger.AppLog.Warnln("can not send:", err)
			}
			logger.AppLog.Infoln("send Request message")
			response, err := stream.Recv()
			if err != nil {
				logger.AppLog.Errorln("response from server: error", err)
				b.state.Store(false)
			} else {
				logger.AppLog.Infof("init Response from Server %s server: %s", response.AmfId, response.VerboseMsg)
				b.state.Store(true)
			}
			return true
		})
		break
	}
	close(initDone)

	if b.state.Load() {
		b.readDone = make(chan struct{})
		go b.connectionOnState()
		go b.readFromServer()
	}
}

func (b *GrpcServer) readFromServer() {
	defer close(b.readDone)
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
				sctplbSelf := sctplbCtx.Sctplb_Self()
				sctplbSelf.Lock()
				backends := make([]sctplbCtx.NF, len(sctplbSelf.Backends))
				copy(backends, sctplbSelf.Backends)
				sctplbSelf.Unlock()

				var found bool
				for _, instance := range backends {
					b1 := instance.(*GrpcServer)
					if b1.address == response.RedirectId {
						if !b1.state.Load() {
							logger.GrpcLog.Infoln("backend state is not in READY state, so not forwarding redirected Msg")
						} else {
							t := gClient.SctplbMessage{}
							t.VerboseMsg = "Hello From gNB Message !"
							t.Msgtype = gClient.MsgType_GNB_MSG
							t.SctplbId = os.Getenv("HOSTNAME")
							t.Msg = response.Msg
							t.GnbId = response.GnbId
							b1.sendMu.Lock()
							err := b1.stream.Send(&t)
							b1.sendMu.Unlock()
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
				var ran *sctplbCtx.Ran
				// fetch ran connection based on GnbId
				if response.GnbId == "" {
					logger.RanLog.Infoln("received null GnbId from backend NF")
				} else if response.GnbIpAddr != "" {
					// GnbId may present NGSetupreponse/failure receives from NF
					ran, _ = sctplbCtx.Sctplb_Self().RanFindByGnbIp(response.GnbIpAddr)
					if ran != nil && response.GnbId != "" {
						ran.SetRanId(response.GnbId)
						logger.RanLog.Infof("received GnbId: %v for GNbIpAddress: %v from NF", response.GnbId, response.GnbIpAddr)
					}
				} else if response.GnbId != "" {
					ran, _ = sctplbCtx.Sctplb_Self().RanFindByGnbId(response.GnbId)
				}
				if ran != nil {
					_, err := ran.Conn.Write(response.Msg)
					if err != nil {
						logger.RanLog.Infof("err %+v", err)
					}
				} else {
					logger.RanLog.Infof("couldn't fetch sctp connection with GnbId: %v", response.GnbId)
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
			change := b.conn.WaitForStateChange(context.Background(), b.conn.GetState())
			state := b.conn.GetState()
			// Shutdown is terminal (e.g. after Close), so it must end this loop too:
			// otherwise the next WaitForStateChange call never returns.
			if change && (state == connectivity.Idle || state == connectivity.Shutdown) {
				deleteBackendNF(b)
				return
			}
		}
	}()
}

func (b *GrpcServer) Send(msg []byte, end bool, ran *sctplbCtx.Ran) error {
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
		// send GnbId to backendNF if exist
		// GnbIp to backend ig GnbId is not exist, mostly this is for NGSetup Message
		if ran.RanId != nil {
			t.GnbId = *ran.RanId
		} else {
			t.GnbIpAddr = ran.Conn.RemoteAddr().String()
		}
		t.Msg = msg
	}
	b.sendMu.Lock()
	defer b.sendMu.Unlock()
	return b.stream.Send(&t)
}

func (b *GrpcServer) State() bool {
	return b.state.Load()
}

// Close drains the backend stream: it half-closes the send side, then waits
// for readFromServer to finish or ctx to expire, whichever comes first, and
// only force-closes the connection if the drain did not finish in time.
func (b *GrpcServer) Close(ctx context.Context) {
	b.state.Store(false)

	// sendMu may be held by a Send blocked on gRPC flow control, so acquiring
	// it runs in its own goroutine: ctx bounds how long Close waits for it,
	// not how long the lock itself takes.
	sendClosed := make(chan struct{})
	go func() {
		b.sendMu.Lock()
		defer b.sendMu.Unlock()
		if b.stream != nil {
			if err := b.stream.CloseSend(); err != nil {
				logger.GrpcLog.Warnf("close stream error: %+v", err)
			}
		}
		close(sendClosed)
	}()

	select {
	case <-sendClosed:
	case <-ctx.Done():
		logger.GrpcLog.Warnf("timed out closing send side of stream from %s", b.address)
	}

	if b.readDone != nil {
		select {
		case <-b.readDone:
		case <-ctx.Done():
			logger.GrpcLog.Warnf("timed out draining stream from %s, forcing close", b.address)
		}
	}

	// Cancel the stream's own context now that the graceful drain finished or
	// timed out, then tear down the connection.
	if b.streamCancel != nil {
		b.streamCancel()
	}

	if b.conn != nil {
		if err := b.conn.Close(); err != nil {
			logger.GrpcLog.Warnf("close connection error: %+v", err)
		}
	}
}
