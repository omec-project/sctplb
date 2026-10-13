// SPDX-FileCopyrightText: 2022 Open Networking Foundation <info@opennetworking.org>
// SPDX-FileCopyrightText: 2024 Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"context"
	"encoding/hex"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/ishidawataru/sctp"
	"github.com/omec-project/ngap/v2"
	"github.com/omec-project/sctplb/logger"
)

type SCTPHandler struct {
	HandleMessage      func(conn *sctp.SCTPConn, msg []byte)
	HandleNotification func(conn *sctp.SCTPConn, notificationData []byte)
}

const readBufSize uint32 = 8192

// set default read timeout to 2 seconds
var readTimeout syscall.Timeval = syscall.Timeval{Sec: 2, Usec: 0}

var (
	sctpListener *sctp.SCTPListener
	listenerMu   sync.Mutex
	// acceptLoopDone is closed once listenAndServe's accept loop has returned,
	// so Shutdown knows no further connections will be accepted or registered
	// in wg before it waits on wg.
	acceptLoopDone = make(chan struct{})
	connections    sync.Map
	wg             sync.WaitGroup
	shuttingDown   atomic.Bool
)

var handler SCTPHandler

var sctpConfig sctp.SocketConfig = sctp.SocketConfig{
	InitMsg: sctp.InitMsg{
		NumOstreams:    3,
		MaxInstreams:   5,
		MaxAttempts:    2,
		MaxInitTimeout: 2,
	},
	NotificationHandler: func(notificationData []byte) error {
		logger.SctpLog.Debugf("received SCTP notification of size %d bytes", len(notificationData))

		if handler.HandleNotification != nil {
			handler.HandleNotification(nil, notificationData)
		}
		return nil
	},
}

func ServiceRun(addresses []string, port int) {
	logger.AppLog.Infoln("service Run is called")
	handler = SCTPHandler{
		HandleMessage:      dispatchMessage,
		HandleNotification: handleNotification,
	}

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
	defer close(acceptLoopDone)

	listener, err := sctpConfig.Listen("sctp", addr)
	if err != nil {
		logger.SctpLog.Errorf("failed to listen: %+v", err)
		return
	}

	// Publish the listener and check for a concurrent shutdown under the same
	// lock Shutdown uses, so the listener can never be missed nor left open.
	listenerMu.Lock()
	if shuttingDown.Load() {
		listenerMu.Unlock()
		if closeErr := listener.Close(); closeErr != nil {
			logger.SctpLog.Warnf("close listener error: %+v", closeErr)
		}
		return
	}
	sctpListener = listener
	listenerMu.Unlock()

	logger.SctpLog.Infof("listen on %s", sctpListener.Addr())

	for {
		newConn, err := sctpListener.AcceptSCTP()
		if err != nil {
			if shuttingDown.Load() {
				logger.SctpLog.Infoln("SCTP listener closed, stopping accept loop")
				return
			}
			switch err {
			case syscall.EINTR, syscall.EAGAIN:
				logger.SctpLog.Debugf("acceptSCTP: %+v", err)
			default:
				logger.SctpLog.Errorf("failed to accept: %+v", err)
			}
			continue
		}

		var info *sctp.SndRcvInfo
		if infoTmp, getParamErr := newConn.GetDefaultSentParam(); getParamErr != nil {
			logger.SctpLog.Errorf("get default sent param error: %+v, accept failed", getParamErr)
			if closeErr := newConn.Close(); closeErr != nil {
				logger.SctpLog.Errorf("close error: %+v", closeErr)
			}
			continue
		} else {
			info = infoTmp
			logger.SctpLog.Debugf("get default sent param[value: %+v]", info)
		}

		info.PPID = ngap.PPID
		if setParamErr := newConn.SetDefaultSentParam(info); setParamErr != nil {
			logger.SctpLog.Errorf("set default sent param error: %+v, accept failed", setParamErr)
			if closeErr := newConn.Close(); closeErr != nil {
				logger.SctpLog.Errorf("close error: %+v", closeErr)
			}
			continue
		} else {
			logger.SctpLog.Debugf("set default sent param[value: %+v]", info)
		}

		events := sctp.SCTP_EVENT_DATA_IO | sctp.SCTP_EVENT_SHUTDOWN | sctp.SCTP_EVENT_ASSOCIATION
		if subErr := newConn.SubscribeEvents(events); subErr != nil {
			logger.SctpLog.Errorf("failed to accept: %+v", subErr)
			if closeErr := newConn.Close(); closeErr != nil {
				logger.SctpLog.Errorf("close error: %+v", closeErr)
			}
			continue
		} else {
			logger.SctpLog.Debugln("subscribe SCTP event[DATA_IO, SHUTDOWN_EVENT, ASSOCIATION_CHANGE]")
		}

		if setBufErr := newConn.SetReadBuffer(int(readBufSize)); setBufErr != nil {
			logger.SctpLog.Errorf("set read buffer error: %+v, accept failed", setBufErr)
			if closeErr := newConn.Close(); closeErr != nil {
				logger.SctpLog.Errorf("close error: %+v", closeErr)
			}
			continue
		} else {
			logger.SctpLog.Debugf("set read buffer to %d bytes", readBufSize)
		}

		// Set read timeout using SO_RCVTIMEO socket option
		// This is the proper way to set timeouts on SCTP sockets
		rawConn, err := newConn.SyscallConn()
		if err != nil {
			logger.SctpLog.Errorf("get syscall conn error: %+v, accept failed", err)
			if err = newConn.Close(); err != nil {
				logger.SctpLog.Errorf("close error: %+v", err)
			}
			continue
		}

		var setTimeoutErr error
		err = rawConn.Control(func(fd uintptr) {
			setTimeoutErr = syscall.SetsockoptTimeval(int(fd), syscall.SOL_SOCKET, syscall.SO_RCVTIMEO, &readTimeout)
		})
		if err != nil || setTimeoutErr != nil {
			logger.SctpLog.Errorf("set read timeout error: control=%+v, setsockopt=%+v, accept failed", err, setTimeoutErr)
			if err = newConn.Close(); err != nil {
				logger.SctpLog.Errorf("close error: %+v", err)
			}
			continue
		} else {
			logger.SctpLog.Debugf("set read timeout: %+v", readTimeout)
		}

		logger.SctpLog.Infof("[AMF] SCTP Accept from: %s", newConn.RemoteAddr().String())
		peer := &SctpConnections{}
		peer.conn = newConn
		peer.address = newConn.RemoteAddr().String()
		connections.Store(newConn, peer)

		// Register before launching so a concurrent wg.Wait in Shutdown can never
		// observe a zero counter for a connection that was already accepted.
		wg.Go(func() {
			handleConnection(newConn, readBufSize, handler)
		})
	}
}

func handleConnection(conn *sctp.SCTPConn, bufsize uint32, handler SCTPHandler) {
	buf := make([]byte, bufsize)

	defer func() {
		connections.Delete(conn)

		// The fd may already be closed by lower-level socket state transitions.
		if err := conn.Close(); err != nil && err != syscall.EBADF {
			logger.SctpLog.Errorf("close connection error: %+v", err)
		}
		logger.SctpLog.Infof("connection[addr: %+v] closed", conn.RemoteAddr())
	}()

	for {
		n, info, err := conn.SCTPRead(buf)
		if err != nil {
			switch err {
			case io.EOF, io.ErrUnexpectedEOF:
				logger.SctpLog.Debugf("connection[addr: %+v] closed by peer (EOF)", conn.RemoteAddr())
				return
			case syscall.EAGAIN:
				logger.SctpLog.Debugln("SCTP read timeout")
				// Timeout is set via SO_RCVTIMEO socket option, no need to reset
				continue
			case syscall.EINTR:
				logger.SctpLog.Debugf("SCTPRead interrupted: %+v", err)
				continue
			case syscall.ECONNRESET:
				logger.SctpLog.Infof("connection[addr: %+v] reset by peer", conn.RemoteAddr())
				return
			case syscall.ENOTCONN:
				logger.SctpLog.Infof("connection[addr: %+v] not connected", conn.RemoteAddr())
				return
			default:
				logger.SctpLog.Errorf("handle connection [addr: %+v] error: %+v", conn.RemoteAddr(), err)
				return
			}
		}

		// Check if this is a notification (MSG_NOTIFICATION flag)
		if info != nil && (info.Flags&sctp.MSG_NOTIFICATION) != 0 {
			logger.SctpLog.Debugf("received connection-specific SCTP notification")
			if handler.HandleNotification != nil {
				handler.HandleNotification(conn, buf[:n])
			}
			continue
		}

		// Regular message handling
		if info == nil {
			logger.SctpLog.Warnf("received SCTP message with nil SndRcvInfo, discarding packet")
			continue
		}

		if info.PPID != ngap.PPID {
			logger.SctpLog.Warnf("received SCTP PPID %d != %d (expected NGAP), discarding packet",
				info.PPID, ngap.PPID)
			continue
		}

		// Validate data length
		if n <= 0 {
			logger.SctpLog.Warnf("received empty SCTP packet, discarding")
			continue
		}

		logger.SctpLog.Debugf("read %d bytes", n)
		logger.SctpLog.Debugf("packet content: %+v", hex.Dump(buf[:n]))

		handler.HandleMessage(conn, buf[:n])
	}
}

// Shutdown stops accepting new SCTP connections, closes the existing gNB and
// backend NF connections, and waits for their handler goroutines to finish or
// for ctx to expire, whichever happens first.
func Shutdown(ctx context.Context) {
	listenerMu.Lock()
	shuttingDown.Store(true)
	listener := sctpListener
	listenerMu.Unlock()

	if listener != nil {
		if err := listener.Close(); err != nil {
			logger.SctpLog.Warnf("close listener error: %+v", err)
		}
	}

	// Wait for the accept loop to stop so every accepted connection has
	// already registered with wg before we close connections and wait on it.
	select {
	case <-acceptLoopDone:
	case <-ctx.Done():
		logger.AppLog.Warnln("timed out waiting for accept loop to stop")
	}

	connections.Range(func(key, _ any) bool {
		conn := key.(*sctp.SCTPConn)
		if err := conn.Close(); err != nil && err != syscall.EBADF {
			logger.SctpLog.Warnf("close connection error: %+v", err)
		}
		return true
	})

	// Wait for SCTP handlers to finish before closing backend streams: a
	// handler that already selected a backend but hasn't sent yet must not
	// lose its message to a concurrent CloseSend. On timeout, closeBackends
	// can still force-unblock a stuck send.
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		logger.AppLog.Infoln("all connections closed")
	case <-ctx.Done():
		logger.AppLog.Warnln("timed out waiting for connections to close")
	}

	closeBackends(ctx)
}
