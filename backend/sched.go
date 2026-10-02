// SPDX-FileCopyrightText: 2022 Open Networking Foundation <info@opennetworking.org>
// SPDX-FileCopyrightText: 2024 Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package backend

import (
	stdctx "context"
	"encoding/binary"
	"net"
	"time"

	"github.com/ishidawataru/sctp"
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

// discoveryInterval is the pause between two passes over the configured
// services.
const discoveryInterval = 2 * time.Second

// lookupIPAddr resolves a service name, and startBackend connects a newly found
// backend; both are variables so that tests can replace them.
var (
	lookupIPAddr = net.DefaultResolver.LookupIPAddr
	startBackend = func(backend context.NF, port int) { go backend.ConnectToServer(port) }
)

// DispatchAddServer keeps the backend pool in step with the configured
// services: every discoveryInterval, each service's name is resolved once and
// a backend is added for every IPv4 address that is not one yet. There can be
// more than one message outstanding towards the same backend.
func (b BackendSvc) DispatchAddServer() {
	for {
		for _, svc := range b.Cfg.Configuration.Services {
			b.discoverService(svc.Uri)
		}
		time.Sleep(discoveryInterval)
	}
}

// discoverService resolves one service once. A failed lookup is logged, and the
// next pass tries again.
func (b BackendSvc) discoverService(uri string) {
	logger.DiscoveryLog.Debugln("discover service", uri)
	ips, err := lookupIPAddr(stdctx.Background(), uri)
	if err != nil {
		logger.DiscoveryLog.Warnf("discover service %s error %+v", uri, err)
		return
	}
	ctx := context.Sctplb_Self()
	for _, ipAddr := range ips {
		ipv4 := ipAddr.IP.To4()
		if ipv4 == nil {
			continue
		}
		address := ipv4.String()
		logger.DiscoveryLog.Debugf("discover service %s, ip %s", uri, address)

		ctx.Lock()
		if hasBackend(ctx, address) {
			ctx.Unlock()
			continue
		}
		var backend context.NF
		switch b.Cfg.Configuration.Type {
		case "grpc":
			backend = &GrpcServer{address: address}
		default:
			ctx.Unlock()
			logger.DiscoveryLog.Warnln("unsupported backend type:", b.Cfg.Configuration.Type)
			return
		}
		ctx.AddNF(backend)
		ctx.Unlock()

		logger.DiscoveryLog.Infoln("new server found IPv4:", address)
		startBackend(backend, b.Cfg.Configuration.SctpGrpcPort)
	}
}

// hasBackend reports whether a backend with this address is in the pool. The
// caller holds the context's lock.
func hasBackend(ctx *context.SctplbContext, address string) bool {
	for _, instance := range ctx.Backends {
		if grpcServer, ok := instance.(*GrpcServer); ok && grpcServer.address == address {
			return true
		}
	}
	return false
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

func dispatchMessage(conn *sctp.SCTPConn, msg []byte) {
	// add this message for one of the client
	// select server who can handle this message.. round robin
	// add message in the server queue
	// select the server which is connected

	// Implement rate limit per gNb here
	// implement per site rate limit here
	var peer *SctpConnections
	p, ok := connections.Load(conn)
	if !ok {
		logger.SctpLog.Infoln("SCTP message for unknown connection")
		return
	}
	peer = p.(*SctpConnections)
	logger.SctpLog.Infoln("handle SCTP message from peer", peer.address)

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

func handleNotification(conn *sctp.SCTPConn, notificationData []byte) {
	if conn == nil {
		logger.SctpLog.Infof("handle global SCTP notification")
		handleGlobalSCTPNotification(notificationData)
		return
	}

	sctplbSelf := context.Sctplb_Self()
	logger.SctpLog.Infof("handle SCTP Notification[addr: %+v]", conn.RemoteAddr())

	ran, ok := sctplbSelf.RanFindByConn(conn)
	if !ok {
		logger.SctpLog.Warnf("RAN context has been removed[addr: %+v]", conn.RemoteAddr())
		return
	}

	// Clean up stale connections in SctplbRanPool
	sctplbSelf.RanPool.Range(func(key, value any) bool {
		amfRan := value.(*context.Ran)
		if amfRan.Conn == nil {
			amfRan.Remove()
			ran.Log.Infoln("removed RAN with nil connection from AmfRan pool")
		}
		return true
	})

	// NotificationHeader = Type (2 bytes) + Flags (2 bytes) + Length (4 bytes) = 8 bytes
	if len(notificationData) < 8 {
		ran.Log.Warnf("notification data too short: %d bytes", len(notificationData))
		return
	}

	// Parse notification header using LittleEndian (host byte order)
	notificationType := sctp.SCTPNotificationType(binary.LittleEndian.Uint16(notificationData[0:2]))
	notificationFlags := binary.LittleEndian.Uint16(notificationData[2:4])
	notificationLength := binary.LittleEndian.Uint32(notificationData[4:8])

	ran.Log.Debugf("processing notification - Type: %d, Flags: %d, Length: %d",
		notificationType, notificationFlags, notificationLength)

	// Validate notification length matches actual data
	if uint32(len(notificationData)) < notificationLength {
		ran.Log.Warnf("notification data length mismatch: got %d bytes, expected %d",
			len(notificationData), notificationLength)
		return
	}

	switch notificationType {
	case sctp.SCTP_ASSOC_CHANGE:
		ran.Log.Infoln("SCTP_ASSOC_CHANGE notification")
		// SCTP Association Change Notification Structure:
		// notificationData = Type (2 bytes) + Flags (2 bytes) + Length (4 bytes) +
		// State (2 bytes) + Error (2 bytes) + outboundStreams (2 bytes) +
		// InboundStreams (2 bytes) + AssocID (4 bytes) = 20 bytes
		if len(notificationData) < 20 {
			ran.Log.Warnf("SCTP_ASSOC_CHANGE notification data too short: got %d bytes, need minimum 20",
				len(notificationData))
			return
		}
		state := sctp.SCTPState(binary.LittleEndian.Uint16(notificationData[8:10]))
		errorSctp := binary.LittleEndian.Uint16(notificationData[10:12])
		outboundStreams := binary.LittleEndian.Uint16(notificationData[12:14])
		inboundStreams := binary.LittleEndian.Uint16(notificationData[14:16])
		assocID := binary.LittleEndian.Uint32(notificationData[16:20])

		ran.Log.Debugf("association change - State: %v, Error: %d, Out: %d, In: %d, AssocID: %d",
			state, errorSctp, outboundStreams, inboundStreams, assocID)

		switch state {
		case sctp.SCTP_COMM_LOST:
			ran.Log.Infoln("SCTP state is SCTP_COMM_LOST, close the connection")
			ran.Remove()
		case sctp.SCTP_SHUTDOWN_COMP:
			ran.Log.Infoln("SCTP state is SCTP_SHUTDOWN_COMP, close the connection")
			ran.Remove()
		case sctp.SCTP_COMM_UP:
			ran.Log.Infoln("SCTP association is up")
		case sctp.SCTP_RESTART:
			ran.Log.Infoln("SCTP association restarted")
		default:
			ran.Log.Warnf("SCTP state[%d] is not handled", state)
		}

	case sctp.SCTP_SHUTDOWN_EVENT:
		ran.Log.Infoln("SCTP_SHUTDOWN_EVENT notification, close the connection")
		ran.Remove()

	case sctp.SCTP_PEER_ADDR_CHANGE:
		ran.Log.Infoln("SCTP_PEER_ADDR_CHANGE notification")

	case sctp.SCTP_REMOTE_ERROR:
		ran.Log.Warnln("SCTP_REMOTE_ERROR notification - peer reported error")

	case sctp.SCTP_SEND_FAILED:
		ran.Log.Warnln("SCTP_SEND_FAILED notification - message delivery failed")

	default:
		ran.Log.Warnf("unhandled notification type: %d", notificationType)
	}
}

func handleGlobalSCTPNotification(notificationHeader []byte) {
	// notificationHeader = Type (2 bytes) + Flags (2 bytes) + Length (4 bytes) = 8 bytes
	if len(notificationHeader) < 8 {
		logger.SctpLog.Warnf("global notification data too short: %d bytes", len(notificationHeader))
		return
	}

	notificationType := sctp.SCTPNotificationType(binary.LittleEndian.Uint16(notificationHeader[0:2]))
	logger.SctpLog.Debugf("handling global SCTP notification of type: %d", notificationType)

	switch notificationType {
	case sctp.SCTP_SHUTDOWN_EVENT:
		logger.SctpLog.Warnln("global SCTP_SHUTDOWN_EVENT notification - listener shutting down")

	case sctp.SCTP_ASSOC_CHANGE:
		logger.SctpLog.Infoln("global SCTP_ASSOC_CHANGE notification")

	default:
		logger.SctpLog.Debugf("global notification type: %d", notificationType)
	}
}
