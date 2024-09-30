// SPDX-FileCopyrightText: 2022 Open Networking Foundation <info@opennetworking.org>
// SPDX-FileCopyrightText: 2024 Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package backend

import (
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
		logger.DispatchLog.Errorln("there are no backend NFs running")
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
				logger.DiscoveryLog.Debugln("discover Service", svc.Uri)
				ips, err := net.LookupIP(svc.Uri)
				if err != nil {
					logger.DiscoveryLog.Warnf("discover Service %s error %+v", svc.Uri, err)
					time.Sleep(2 * time.Second)
					continue
				}
				for _, ip := range ips {
					logger.DiscoveryLog.Debugln("discover Service %s, ip %s", svc.Uri, ", ip", ip.String())
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
						logger.DiscoveryLog.Infoln("new server found IPv4:", ipv4.String())
						var backend context.NF
						switch b.Cfg.Configuration.Type {
						case "grpc":
							backend = &GrpcServer{
								address: ipv4.String(),
							}
						default:
							logger.DiscoveryLog.Warnln("unsupported backend type:", b.Cfg.Configuration.Type)
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
		logger.AppLog.Infof("available backend %v", b1)
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
		logger.SctpLog.Infoln("notification for unknown connection")
		return
	} else {
		peer = p.(*SctpConnections)
		logger.SctpLog.Infoln("handle SCTP Notification from peer", peer.address)
	}
	ctx := context.Sctplb_Self()
	ctx.Lock()
	defer ctx.Unlock()
	ran, _ := ctx.RanFindByConn(conn)
	if len(msg) == 0 {
		logger.SctpLog.Infof("send Gnb connection [%v] close message to all AMF Instances", peer.address)
		if ctx.Backends != nil && ctx.NFLength() > 0 {
			var i int
			for ; i < ctx.NFLength(); i++ {
				backend := ctx.Backends[i]
				if backend.State() {
					if err := backend.Send(msg, true, ran); err != nil {
						logger.SctpLog.Errorln("can not send", err)
					}
				}
			}
		} else {
			logger.SctpLog.Errorln("no AMF Connections")
		}
		context.Sctplb_Self().DeleteRan(conn)
		return
	}
	if ran == nil {
		ran = context.Sctplb_Self().NewRan(conn)
	}
	logger.SctpLog.Infoln("message received from remoteAddr", conn.RemoteAddr().String())
	if ctx.NFLength() == 0 {
		logger.AppLog.Errorln("no backend available")
		return
	}
	var i int
	for ; i < ctx.NFLength(); i++ {
		// Select the backend NF based on RoundRobin Algorithm
		backend := RoundRobin()
		if backend.State() {
			if err := backend.Send(msg, false, ran); err != nil {
				logger.SctpLog.Errorln("can not send:", err)
			}
			break
		}
	}
}
