// SPDX-FileCopyrightText: 2022 Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"fmt"
	"net"
	"time"

	"github.com/omec-project/sctplb/context"
	"github.com/omec-project/sctplb/logger"
)

var next int

type Backend interface {
	State() bool
	Send(msg []byte, b bool, ran *context.Ran) error
}

// returns the backendNF using RoundRobin algorithm
func RoundRobin() Backend {
	ctx := context.Sctplb_Self()
	length := ctx.NFLength()

	if length <= 0 {
		logger.DispatchLog.Errorln("There are no backend NFs running")
		return nil
	}
	if next >= length {
		next = 0
	}

	instance := ctx.Backends[next]
	next++
	return instance
}

func (b BackendSvc) DispatchAddServer() {
	// add server in pool
	// create server
	// create server outstanding message queue
	// connect to server
	// there can be more than 1 message outstanding toards same server
	for {
		ctx := context.Sctplb_Self()
		svcList := b.Cfg.Configuration.Services
		for _, svc := range svcList {
			for {
				logger.DiscoveryLog.Traceln("Discover Service ", svc.Uri)
				ips, err := net.LookupIP(svc.Uri)
				if err != nil {
					logger.DiscoveryLog.Errorln("Discover Service ", svc.Uri, " Error ", err)
					time.Sleep(2 * time.Second)
					continue
				}
				for _, ip := range ips {
					logger.DiscoveryLog.Traceln("Discover Service ", svc.Uri, ", ip ", ip)
					found := false
					if ipv4 := ip.To4(); ipv4 != nil {
						for _, instance := range ctx.Backends {
							b := instance.(*GrpcServer)
							if b.address == ipv4.String() {
								found = true
								break
							}
						}
						if found {
							continue
						}
						logger.DiscoveryLog.Infoln("New Server found IPv4: ", ipv4.String())
						var backend context.NF
						switch b.Cfg.Configuration.Type {
						case "grpc":
							backend = &GrpcServer{
								address: ipv4.String(),
							}
						default:
							logger.DiscoveryLog.Warnln("unsupported backend type: " +
								b.Cfg.Configuration.Type)
						}
						ctx.Lock()
						ctx.AddNF(backend)
						ctx.Unlock()
						go backend.ConnectToServer(b.Cfg.Configuration.SctpGrpcPort)
					}
				}
			}
		}
		time.Sleep(2 * time.Second)
	}
}

func deleteBackendNF(b context.NF) {
	ctx := context.Sctplb_Self()
	ctx.Lock()
	defer ctx.Unlock()
	ctx.DeleteNF(b)
	for _, b1 := range ctx.Backends {
		fmt.Printf("Available backend %v \n", b1)
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
	ctx := context.Sctplb_Self()
	ctx.Lock()
	defer ctx.Unlock()
	ran, _ := ctx.RanFindByConn(conn)
	if len(msg) == 0 {
		logger.SctpLog.Infof("send Gnb connection [%v] close message to all AMF Instances", peer)
		if ctx.Backends != nil && ctx.NFLength() > 0 {
			var i int
			for ; i < ctx.NFLength(); i++ {
				backend := ctx.Backends[i]
				if backend.State() {
					if err := backend.Send(msg, true, ran); err != nil {
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
	if ctx.NFLength() == 0 {
		fmt.Println("NO backend available")
		return
	}
	var i int
	for ; i < ctx.NFLength(); i++ {
		// Select the backend NF based on RoundRobin Algorithm
		backend := RoundRobin()
		if backend.State() {
			if err := backend.Send(msg, false, ran); err != nil {
				logger.SctpLog.Errorln("can not send: ", err)
			}
			break
		}
	}
}
