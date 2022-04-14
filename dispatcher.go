// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"google.golang.org/grpc"
	"log"
	"net"
	"os"
	gClient "github.com/omec-project/sctplb/sdcoreAmfServer"
	"time"
)

type backendNF struct {
	address string
	conn    *grpc.ClientConn
	gc      gClient.NgapServiceClient
	state   bool
	stream  gClient.NgapService_HandleMessageClient
	sctp    net.Conn
}

var backends []*backendNF

func dispatchAddServer(serviceName string) {
	// add server in pool
	// create server
	// create server outstanding message queue
	// connect to server
	// there can be more than 1 message outstanding toards same server

	for {
		discoveryLog.Traceln("Discover Service ", serviceName)
		ips, err := net.LookupIP(serviceName)
		if err != nil {
			discoveryLog.Errorln("Discover Service ", serviceName, " Error ",err)
			time.Sleep(2 * time.Second)
			continue
		}
		for _, ip := range ips {
			discoveryLog.Traceln("Discover Service ", serviceName, ", ip ", ip)
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
				discoveryLog.Infoln("New Server found IPv4: ", ipv4.String())
				backend := &backendNF{}
				backend.address = ipv4.String()
				backends = append(backends, backend)
				go backend.connectToServer()
			}
		}
		time.Sleep(5 * time.Second)
	}
	return
}

func (b *backendNF) readFromServer() {
	for {
		response, err := b.stream.Recv()
		if err != nil {
			log.Printf("Error in Recv %v, Stop listening for this server %v ", err, b.address)
			for i, b1 := range backends {
				if b1 == b {
					backends[i] = backends[len(backends)-1]
					backends = backends[:len(backends)-1]
					break
				}
			}
			for _, b1 := range backends {
				fmt.Println("Available backend %v ", b1)
			}
			return
		} else {
			if response.Msgtype == gClient.MsgType_INIT_MSG {
				log.Printf("Init Response from Server %s server: %s", response.AmfId, response.VerboseMsg)
			} else {
				b.sctp.Write(response.Msg)
			}
		}
	}
}

func (b *backendNF) connectToServer() {
	target := fmt.Sprintf("%s:%d",b.address, SimappConfig.Configuration.SctpGrpcPort)

	fmt.Println("Connecting to target ", target)

	var err error
	b.conn, err = grpc.Dial(target, grpc.WithInsecure())

	if err != nil {
		log.Fatalf("did not connect: %s", err)
	}

	//b.conn = conn
	b.gc = gClient.NewNgapServiceClient(b.conn)

	stream, err := b.gc.HandleMessage(context.Background())
	if err != nil {
		log.Fatalf("openn stream error %v", err)
	}

	b.stream = stream
	for {
		req := gClient.Message{}
		req.VerboseMsg = "Hello From SCTP LB !"
		req.Msgtype = gClient.MsgType_INIT_MSG
		req.SctplbId = os.Getenv("HOSTNAME")

		if err := stream.Send(&req); err != nil {
			log.Fatalf("can not send %v", err)
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
		break
	}
	if b.state == true {
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
		sctpLog.Infof("Notification for unknown connection")
		return
	} else {
		peer = p.(*SctpConnections)
		sctpLog.Warnf("Handle SCTP Notification[addr: %+v], peer %v ", conn.RemoteAddr(), peer)
	}

	if len(msg) == 0 {
		sctpLog.Infof("send Gnb connection close message to AMF %v", peer)
		t := gClient.Message{}
		t.VerboseMsg = "Bye From gNB Message !"
		t.Msgtype = gClient.MsgType_GNB_DISC
		t.SctplbId = os.Getenv("HOSTNAME")
		t.GnbId = peer.address
		t.Msg = msg
		backend := backends[0]
		backend.sctp = conn
		if err := backend.stream.Send(&t); err != nil {
			log.Fatalf("can not send %v", err)
		}
		return
	}
	sctpLog.Println("Message received from remoteAddr ", conn.RemoteAddr().String())
	t := gClient.Message{}
	t.VerboseMsg = "Hello From gNB Message !"
	t.Msgtype = gClient.MsgType_GNB_MSG
	t.SctplbId = os.Getenv("HOSTNAME")
	t.GnbId = conn.RemoteAddr().String()
	t.Msg = msg
	if len(backends) == 0 {
		fmt.Println("NO backend available")
		return
	}
	backend := backends[0]
	backend.sctp = conn
	if err := backend.stream.Send(&t); err != nil {
		log.Fatalf("can not send %v", err)
	}
}
