// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/hex"
	"fmt"
	"git.cs.nctu.edu.tw/calee/sctp"
	"github.com/omec-project/ngap"
	"io"
	"net"
	"sync"
	"syscall"
)

type SCTPHandler struct {
	HandleMessage      func(conn net.Conn, msg []byte)
	HandleNotification func(conn net.Conn, notification sctp.Notification)
}

type SctpConnections struct {
	conn net.Conn
    address string
}

const readBufSize uint32 = 8192

// set default read timeout to 2 seconds
var readTimeout syscall.Timeval = syscall.Timeval{Sec: 2, Usec: 0}

var (
	sctpListener *sctp.SCTPListener
	connections  sync.Map
)

var sctpConfig sctp.SocketConfig = sctp.SocketConfig{
	InitMsg:   sctp.InitMsg{NumOstreams: 3, MaxInstreams: 5, MaxAttempts: 2, MaxInitTimeout: 2},
	RtoInfo:   &sctp.RtoInfo{SrtoAssocID: 0, SrtoInitial: 500, SrtoMax: 1500, StroMin: 100},
	AssocInfo: &sctp.AssocInfo{AsocMaxRxt: 4},
}

func serviceRun(addresses []string, port int) {

	fmt.Println("service Run is called")
	handler := SCTPHandler{
		HandleMessage: dispatchMessage,
	}

	ips := []net.IPAddr{}

	for _, addr := range addresses {
		if netAddr, err := net.ResolveIPAddr("ip", addr); err != nil {
			sctpLog.Errorf("Error resolving address '%s': %v\n", addr, err)
		} else {
			sctpLog.Debugf("Resolved address '%s' to %s\n", addr, netAddr)
			ips = append(ips, *netAddr)
		}
	}

	addr := &sctp.SCTPAddr{
		IPAddrs: ips,
		Port:    port,
	}

	go listenAndServe(addr, handler)
}

func listenAndServe(addr *sctp.SCTPAddr, handler SCTPHandler) {
	if listener, err := sctpConfig.Listen("sctp", addr); err != nil {
		sctpLog.Errorf("Failed to listen: %+v", err)
		return
	} else {
		sctpListener = listener
	}

	sctpLog.Infof("Listen on %s", sctpListener.Addr())

	for {
		newConn, err := sctpListener.AcceptSCTP()
		if err != nil {
			switch err {
			case syscall.EINTR, syscall.EAGAIN:
				sctpLog.Debugf("AcceptSCTP: %+v", err)
			default:
				sctpLog.Errorf("Failed to accept: %+v", err)
			}
			continue
		}

		var info *sctp.SndRcvInfo
		if infoTmp, err := newConn.GetDefaultSentParam(); err != nil {
			sctpLog.Errorf("Get default sent param error: %+v, accept failed", err)
			if err = newConn.Close(); err != nil {
				sctpLog.Errorf("Close error: %+v", err)
			}
			continue
		} else {
			info = infoTmp
			sctpLog.Debugf("Get default sent param[value: %+v]", info)
		}

		info.PPID = ngap.PPID
		if err := newConn.SetDefaultSentParam(info); err != nil {
			sctpLog.Errorf("Set default sent param error: %+v, accept failed", err)
			if err = newConn.Close(); err != nil {
				sctpLog.Errorf("Close error: %+v", err)
			}
			continue
		} else {
			sctpLog.Debugf("Set default sent param[value: %+v]", info)
		}

		events := sctp.SCTP_EVENT_DATA_IO | sctp.SCTP_EVENT_SHUTDOWN | sctp.SCTP_EVENT_ASSOCIATION
		if err := newConn.SubscribeEvents(events); err != nil {
			sctpLog.Errorf("Failed to accept: %+v", err)
			if err = newConn.Close(); err != nil {
				sctpLog.Errorf("Close error: %+v", err)
			}
			continue
		} else {
			sctpLog.Debugln("Subscribe SCTP event[DATA_IO, SHUTDOWN_EVENT, ASSOCIATION_CHANGE]")
		}

		if err := newConn.SetReadBuffer(int(readBufSize)); err != nil {
			sctpLog.Errorf("Set read buffer error: %+v, accept failed", err)
			if err = newConn.Close(); err != nil {
				sctpLog.Errorf("Close error: %+v", err)
			}
			continue
		} else {
			sctpLog.Debugf("Set read buffer to %d bytes", readBufSize)
		}

		if err := newConn.SetReadTimeout(readTimeout); err != nil {
			sctpLog.Errorf("Set read timeout error: %+v, accept failed", err)
			if err = newConn.Close(); err != nil {
				sctpLog.Errorf("Close error: %+v", err)
			}
			continue
		} else {
			sctpLog.Debugf("Set read timeout: %+v", readTimeout)
		}

		sctpLog.Infof("[AMF] SCTP Accept from: %s", newConn.RemoteAddr().String())
		peer := &SctpConnections{}
		peer.conn = newConn
        peer.address = newConn.RemoteAddr().String()
		connections.Store(newConn, peer)

		go handleConnection(newConn, readBufSize, handler)
	}
}

func handleConnection(conn *sctp.SCTPConn, bufsize uint32, handler SCTPHandler) {

	defer func() {
		// if AMF call Stop(), then conn.Close() will return EBADF because conn has been closed inside Stop()
		if err := conn.Close(); err != nil && err != syscall.EBADF {
			sctpLog.Errorf("close connection error: %+v", err)
		}
		connections.Delete(conn)
	}()

	var GnbConnChan chan bool
	GnbConnChan = make(chan bool)

	go func() {
		for {
			buf := make([]byte, bufsize)

			n, info, notification, err := conn.SCTPRead(buf)
			if err != nil {
				switch err {
				case io.EOF, io.ErrUnexpectedEOF:
					sctpLog.Debugln("Read EOF from client")
					GnbConnChan <- false
					return
				case syscall.EAGAIN:
					sctpLog.Debugln("SCTP read timeout")
					continue
				case syscall.EINTR:
					sctpLog.Debugf("SCTPRead: %+v", err)
					continue
				default:
					sctpLog.Errorf("Handle connection[addr: %+v] error: %+v", conn.RemoteAddr(), err)
					GnbConnChan <- false
					return
				}
			}

			if notification != nil {
				p, ok := connections.Load(conn)
				if !ok {
					sctpLog.Infof("Notification for unknown connection")
				} else {
					peer := p.(*SctpConnections)
					sctpLog.Warnf("Handle SCTP Notification[addr: %+v], peer %v ", conn.RemoteAddr(), peer)
					GnbConnChan <- false
				}
			} else {
				if info == nil || info.PPID != ngap.PPID {
					sctpLog.Warnln("Received SCTP PPID != 60, discard this packet")
					continue
				}

				sctpLog.Tracef("Read %d bytes", n)
				sctpLog.Tracef("Packet content:\n%+v", hex.Dump(buf[:n]))

				handler.HandleMessage(conn, buf[:n])
			}
		}
	}()

	for {
		select {
		case x := <-GnbConnChan:
			sctpLog.Errorln("Closing gnb Connection  ", x)
			buf := make([]byte, bufsize)
			handler.HandleMessage(conn, buf[:0])
			return
		}
	}
}
