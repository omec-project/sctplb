// SPDX-FileCopyrightText: 2022 Open Networking Foundation <info@opennetworking.org>
// SPDX-FileCopyrightText: 2024 Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package backend

import (
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
