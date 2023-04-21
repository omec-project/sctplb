// SPDX-FileCopyrightText: 2022 Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	ctxt "context"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"github.com/omec-project/sctplb/context"
	"github.com/omec-project/sctplb/logger"
	gClient "github.com/omec-project/sctplb/sdcoreAmfServer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
)

type backendNF struct {
	address string
	conn    *grpc.ClientConn
	gc      gClient.NgapServiceClient
	state   bool
	stream  gClient.NgapService_HandleMessageClient
}

var (
	backends []*backendNF
	nfNum    int
	next     int
	mutex    sync.Mutex
)

// returns the backendNF using RoundRobin algorithm
func RoundRobin() (nf *backendNF) {
	mutex.Lock()
	defer mutex.Unlock()

	if nfNum <= 0 {
		logger.DispatchLog.Errorln("There are no backend NFs running")
		return nil
	}
	if next >= nfNum {
		next = 0
	}

	nf = backends[next]
	next++
	return nf
}

func dispatchAddServer(serviceName string) {
	// add server in pool
	// create server
	// create server outstanding message queue
	// connect to server
	// there can be more than 1 message outstanding toards same server

	for {
		logger.DiscoveryLog.Traceln("Discover Service ", serviceName)
		ips, err := net.LookupIP(serviceName)
		if err != nil {
			logger.DiscoveryLog.Errorln("Discover Service ", serviceName, " Error ", err)
			time.Sleep(2 * time.Second)
			continue
		}
		for _, ip := range ips {
			logger.DiscoveryLog.Traceln("Discover Service ", serviceName, ", ip ", ip)
			found := false
			if ipv4 := ip.To4(); ipv4 != nil {
				for _, b := range backends {
					if b.address == ipv4.String() {
						found = true
						break
					}
				}
				if found == true {
					continue
				}
				logger.DiscoveryLog.Infoln("New Server found IPv4: ", ipv4.String())
				backend := &backendNF{}
				backend.address = ipv4.String()
				mutex.Lock()
				backends = append(backends, backend)
				nfNum++
				mutex.Unlock()
				go backend.connectToServer()
			}
		}
		time.Sleep(2 * time.Second)
	}
	return
}
func (b *backendNF) connectionOnState() {

	go func() {

		// continue checking for state change
		// until one of break states is found
		for {
			change := b.conn.WaitForStateChange(ctxt.Background(), b.conn.GetState())
			if change && b.conn.GetState() == connectivity.Idle {
				b.deleteBackendNF()
				return
			}

		}
	}()

}

func (b *backendNF) deleteBackendNF() {
	mutex.Lock()
	defer mutex.Unlock()

	for i, b1 := range backends {
		if b1 == b {
			backends[i] = backends[len(backends)-1]
			backends = backends[:len(backends)-1]
			mutex.Lock()
			nfNum--
			mutex.Unlock()
			break
		}
	}
	for _, b1 := range backends {
		fmt.Println("Available backend %v ", b1)
	}
}

func (b *backendNF) readFromServer() {
	for {
		response, err := b.stream.Recv()
		if err != nil {
			log.Printf("Error in Recv %v, Stop listening for this server %v ", err, b.address)
			b.deleteBackendNF()
			return
		} else {
			if response.Msgtype == gClient.MsgType_INIT_MSG {
				log.Printf("Init Response from Server %s server: %s", response.AmfId, response.VerboseMsg)
			} else if response.Msgtype == gClient.MsgType_REDIRECT_MSG {
				var found bool
				for _, b1 := range backends {
					if b1.address == response.RedirectId {
						if b1.state == false {
							log.Printf("backend state is not in READY state, so not forwarding redirected Msg")
						} else {
							t := gClient.SctplbMessage{}
							t.VerboseMsg = "Hello From gNB Message !"
							t.Msgtype = gClient.MsgType_GNB_MSG
							t.SctplbId = os.Getenv("HOSTNAME")
							t.Msg = response.Msg
							t.GnbId = response.GnbId
							b1.stream.Send(&t)
							log.Printf("successfully forwarded msg to correct AMF")
							found = true
						}
						break
					}
				}
				if !found {
					log.Printf("dropping redirected message as backend ip [%v] is not exist", response.RedirectId)
				}

			} else {
				var ran *context.Ran
				//fetch ran connection based on GnbId
				if response.GnbId == "" {
					log.Printf("Received null GnbId from backend NF")
				} else if response.GnbIpAddr != "" {
					// GnbId may present NGSetupreponse/failure receives from NF
					ran, _ = context.Sctplb_Self().RanFindByGnbIp(response.GnbIpAddr)
					if ran != nil && response.GnbId != "" {
						ran.SetRanId(response.GnbId)
						log.Printf("Received GnbId: %v for GNbIpAddress: %v from NF", response.GnbId, response.GnbIpAddr)
					}
				} else if response.GnbId != "" {
					ran, _ = context.Sctplb_Self().RanFindByGnbId(response.GnbId)
				}
				if ran != nil {
					ran.Conn.Write(response.Msg)
				} else {
					log.Printf("Couldn't fetch sctp connection with GnbId: %v", response.GnbId)
				}
			}
		}
	}
}

func (b *backendNF) connectToServer() {
	target := fmt.Sprintf("%s:%d", b.address, SimappConfig.Configuration.SctpGrpcPort)

	fmt.Println("Connecting to target ", target)

	var err error
	b.conn, err = grpc.Dial(target, grpc.WithInsecure())

	if err != nil {
		fmt.Println("did not connect: ", err)
		b.deleteBackendNF()
		return
	}

	//b.conn = conn
	b.gc = gClient.NewNgapServiceClient(b.conn)

	stream, err := b.gc.HandleMessage(ctxt.Background())
	if err != nil {
		log.Println("openn stream error ", err)
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
				log.Printf("ran connection %v is exist without GnbId, so not sending this ran details to NF",
					candidate.GnbIp)
				//return true
			}
			if err := stream.Send(&req); err != nil {
				log.Println("can not send: ", err)
			}
			log.Printf("Send Request message : %v", req)
			response, err := stream.Recv()
			if err != nil {
				log.Println("Response from server: error ", err)
				b.state = false
			} else {
				log.Printf("Init Response from Server %s server: %s", response.AmfId, response.VerboseMsg)
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

func dispatchMessage(conn net.Conn, msg []byte) { //*gClient.Message) {
	// add this message for one of the client
	// select server who can handle this message.. round robin
	// add message in the server queue
	// select the server which is connected

	// Implement rate limit per gNb here
	// implement per site rate limit here
	var peer *SctpConnections
	p, ok := connections.Load(conn)
	if !ok {
		logger.SctpLog.Infof("Notification for unknown connection")
		return
	} else {
		peer = p.(*SctpConnections)
		logger.SctpLog.Warnf("Handle SCTP Notification[addr: %+v], peer %v ", conn.RemoteAddr(), peer)
	}

	ran, _ := context.Sctplb_Self().RanFindByConn(conn)
	if len(msg) == 0 {
		logger.SctpLog.Infof("send Gnb connection [%v] close message to all AMF Instances", peer)
		t := gClient.SctplbMessage{}
		t.VerboseMsg = "Bye From gNB Message !"
		t.Msgtype = gClient.MsgType_GNB_DISC
		t.SctplbId = os.Getenv("HOSTNAME")
		if ran != nil && ran.RanId != nil {
			t.GnbId = *ran.RanId
		}
		t.Msg = msg
		if backends != nil && len(backends) > 0 {
			var i int
			for ; i < len(backends); i++ {
				backend := backends[i]
				if backend.state == true {
					if err := backend.stream.Send(&t); err != nil {
						logger.SctpLog.Errorln("can not send ", err)
					}
				}
			}
		} else {
			logger.SctpLog.Errorln("No AMF Connections")
		}
		context.Sctplb_Self().DeleteRan(conn)
		return
	}
	if ran == nil {
		ran = context.Sctplb_Self().NewRan(conn)
	}
	logger.SctpLog.Println("Message received from remoteAddr ", conn.RemoteAddr().String())
	t := gClient.SctplbMessage{}
	t.VerboseMsg = "Hello From gNB Message !"
	t.Msgtype = gClient.MsgType_GNB_MSG
	t.SctplbId = os.Getenv("HOSTNAME")
	//send GnbId to backendNF if exist
	//GnbIp to backend ig GnbId is not exist, mostly this is for NGSetup Message
	if ran.RanId != nil {
		t.GnbId = *ran.RanId
	} else {
		t.GnbIpAddr = conn.RemoteAddr().String()
	}
	t.Msg = msg
	if len(backends) == 0 {
		fmt.Println("NO backend available")
		return
	}
	var i int
	for ; i < len(backends); i++ {
		//Select the backend NF based on RoundRobin Algorithm
		backend := RoundRobin()
		if backend.state == true {
			if err := backend.stream.Send(&t); err != nil {
				logger.SctpLog.Errorln("can not send: ", err)
			}
			break
		}
	}
}
