// SPDX-FileCopyrightText: 2022 Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

package backend

import (
	ctxt "context"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/omec-project/sctplb/config"
	"github.com/omec-project/sctplb/context"
	"github.com/omec-project/sctplb/logger"
	gClient "github.com/omec-project/sctplb/sdcoreAmfServer"
	"google.golang.org/grpc/connectivity"
)

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

type BackendSvc struct {
	Cfg config.Config
}

func (b BackendSvc) Run() {
	// add server in pool
	// create server
	// create server outstanding message queue
	// connect to server
	// there can be more than 1 message outstanding toards same server
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
					go backend.connectToServer(b.Cfg.Configuration.SctpGrpcPort)
				}
			}
			time.Sleep(2 * time.Second)
		}
	}
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
