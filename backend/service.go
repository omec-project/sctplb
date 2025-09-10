// SPDX-FileCopyrightText: 2022 Open Networking Foundation <info@opennetworking.org>
// SPDX-FileCopyrightText: 2024 Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"encoding/hex"
	"io"
	"net"
	"sync"
	"syscall"

	"github.com/ishidawataru/sctp"
	"github.com/omec-project/ngap"
	"github.com/omec-project/sctplb/logger"
)

type SCTPHandler func(conn net.Conn, msg []byte)

const readBufSize uint32 = 8192

var (
	sctpListener *sctp.SCTPListener
	connections  sync.Map
)

var sctpConfig sctp.SocketConfig = sctp.SocketConfig{
	InitMsg: sctp.InitMsg{NumOstreams: 3, MaxInstreams: 5, MaxAttempts: 2, MaxInitTimeout: 2},
}

func ServiceRun(addresses []string, port int) {
	logger.AppLog.Infoln("service Run is called")
	handler := SCTPHandler(dispatchMessage)

	ips := []net.IPAddr{}

	for _, addr := range addresses {
		if netAddr, err := net.ResolveIPAddr("ip", addr); err != nil {
			logger.SctpLog.Errorf("error resolving address '%s': %v", addr, err)
		} else {
			logger.SctpLog.Debugf("resolved address '%s' to %s", addr, netAddr)
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
	listener, err := sctpConfig.Listen("sctp", addr)
	if err != nil {
		logger.SctpLog.Errorf("failed to listen: %+v", err)
		return
	}
	sctpListener = listener

	logger.SctpLog.Infof("listen on %s", sctpListener.Addr())

	for {
		newConn, err := sctpListener.AcceptSCTP()
		if err != nil {
			switch err {
			case syscall.EINTR, syscall.EAGAIN:
				logger.SctpLog.Debugf("acceptSCTP: %+v", err)
			default:
				logger.SctpLog.Errorf("failed to accept: %+v", err)
			}
			continue
		}

		var info *sctp.SndRcvInfo
		if infoTmp, err := newConn.GetDefaultSentParam(); err != nil {
			logger.SctpLog.Errorf("get default sent param error: %+v, accept failed", err)
			if err = newConn.Close(); err != nil {
				logger.SctpLog.Errorf("close error: %+v", err)
			}
			continue
		} else {
			info = infoTmp
			logger.SctpLog.Debugf("get default sent param[value: %+v]", info)
		}

		info.PPID = ngap.PPID
		if err := newConn.SetDefaultSentParam(info); err != nil {
			logger.SctpLog.Errorf("set default sent param error: %+v, accept failed", err)
			if err = newConn.Close(); err != nil {
				logger.SctpLog.Errorf("close error: %+v", err)
			}
			continue
		} else {
			logger.SctpLog.Debugf("set default sent param[value: %+v]", info)
		}

		events := sctp.SCTP_EVENT_DATA_IO | sctp.SCTP_EVENT_SHUTDOWN | sctp.SCTP_EVENT_ASSOCIATION
		if err := newConn.SubscribeEvents(events); err != nil {
			logger.SctpLog.Errorf("failed to accept: %+v", err)
			if err = newConn.Close(); err != nil {
				logger.SctpLog.Errorf("close error: %+v", err)
			}
			continue
		} else {
			logger.SctpLog.Debugln("subscribe SCTP event[DATA_IO, SHUTDOWN_EVENT, ASSOCIATION_CHANGE]")
		}

		if err := newConn.SetReadBuffer(int(readBufSize)); err != nil {
			logger.SctpLog.Errorf("set read buffer error: %+v, accept failed", err)
			if err = newConn.Close(); err != nil {
				logger.SctpLog.Errorf("close error: %+v", err)
			}
			continue
		} else {
			logger.SctpLog.Debugf("set read buffer to %d bytes", readBufSize)
		}

		logger.SctpLog.Infof("[AMF] SCTP Accept from: %s", newConn.RemoteAddr().String())
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
			logger.SctpLog.Errorf("close connection error: %+v", err)
		}
		connections.Delete(conn)
	}()

	GnbConnChan := make(chan bool)

	go func() {
		for {
			buf := make([]byte, bufsize)

			n, info, err := conn.SCTPRead(buf)
			if err != nil {
				switch err {
				case io.EOF, io.ErrUnexpectedEOF:
					logger.SctpLog.Debugln("read EOF from client")
					GnbConnChan <- false
					return
				case syscall.EAGAIN:
					logger.SctpLog.Debugln("SCTP read timeout")
					continue
				case syscall.EINTR:
					logger.SctpLog.Debugf("SCTPRead: %+v", err)
					continue
				default:
					logger.SctpLog.Errorf("handle connection [addr: %+v] error: %+v", conn.RemoteAddr(), err)
					GnbConnChan <- false
					return
				}
			}

			if info == nil || info.PPID != ngap.PPID {
				logger.SctpLog.Warnln("received SCTP PPID != 60, discard this packet")
				continue
			}

			logger.SctpLog.Debugf("read %d bytes", n)
			logger.SctpLog.Debugf("packet content: %+v", hex.Dump(buf[:n]))

			handler(conn, buf[:n])
		}
	}()

	for x := range GnbConnChan {
		logger.SctpLog.Infoln("closing gnb Connection:", x)
		buf := make([]byte, bufsize)
		handler(conn, buf[:0])
		return
	}
}
