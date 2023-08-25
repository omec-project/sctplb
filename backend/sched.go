// SPDX-FileCopyrightText: 2022 Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"fmt"
	"net"
	"time"

	"github.com/omec-project/sctplb/config"
	"github.com/omec-project/sctplb/context"
	"github.com/omec-project/sctplb/logger"
)

var (
	next int
)

// returns the backendNF using RoundRobin algorithm
func RoundRobin() (nf *BackendNF) {
	ctx := context.Sctplb_Self()
	len := ctx.NFLength()

	if len <= 0 {
		logger.DispatchLog.Errorln("There are no backend NFs running")
		return nil
	}
	if next >= len {
		next = 0
	}

	instance := ctx.Backends[next]
	nf = instance.(*BackendNF)
	next++
	return nf
}

type BackendSvc struct {
	Cfg config.Config
}

func (b BackendSvc) Run() {
	// add server in pool
	// create server
	// create server outstanding message queue
	// connect to server
	// there can be more than 1 message outstanding toards same server
	ctx := context.Sctplb_Self()
	svcList := b.Cfg.Configuration.ServiceName
	for _, name := range svcList {
		for {
			logger.DiscoveryLog.Traceln("Discover Service ", name)
			ips, err := net.LookupIP(name)
			if err != nil {
				logger.DiscoveryLog.Errorln("Discover Service ", name, " Error ", err)
				time.Sleep(2 * time.Second)
				continue
			}
			for _, ip := range ips {
				logger.DiscoveryLog.Traceln("Discover Service ", name, ", ip ", ip)
				found := false
				if ipv4 := ip.To4(); ipv4 != nil {
					for _, instance := range ctx.Backends {
						b := instance.(*BackendNF)
						if b.address == ipv4.String() {
							found = true
							break
						}
					}
					if found == true {
						continue
					}
					logger.DiscoveryLog.Infoln("New Server found IPv4: ", ipv4.String())
					backend := &BackendNF{}
					backend.address = ipv4.String()
					ctx.Lock()
					ctx.AddNF(backend)
					ctx.Unlock()
					go backend.connectToServer(b.Cfg.Configuration.SctpGrpcPort)
				}
			}
			time.Sleep(2 * time.Second)
		}
	}
}

func (b *BackendNF) deleteBackendNF() {
	ctx := context.Sctplb_Self()
	ctx.Lock()
	defer ctx.Unlock()
	ctx.DeleteNF(b)
	for _, b1 := range ctx.Backends {
		fmt.Printf("Available backend %v \n", b1)
	}
}

type SctpConnections struct {
	conn    net.Conn
	address string
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
				instance := ctx.Backends[i]
				backend := instance.(*BackendNF)
				if backend.state == true {
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
		if backend.state == true {
			if err := backend.Send(msg, false, ran); err != nil {
				logger.SctpLog.Errorln("can not send: ", err)
			}
			break
		}
	}
}
