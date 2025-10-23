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
	"syscall"

	"github.com/ishidawataru/sctp"
	"github.com/omec-project/ngap"
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
	sctpListener   *sctp.SCTPListener
	connections    sync.Map
	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc
	wg             sync.WaitGroup
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

func init() {
	shutdownCtx, shutdownCancel = context.WithCancel(context.Background())
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
	if listener, err := sctpConfig.Listen("sctp", addr); err != nil {
		logger.SctpLog.Errorf("failed to listen: %+v", err)
		return
	} else {
		sctpListener = listener
	}

	logger.SctpLog.Infof("listen on %s", sctpListener.Addr())

	for {
		select {
		case <-shutdownCtx.Done():
			logger.SctpLog.Info("shutting down listener")
			return
		default:
			newConn, err := sctpListener.AcceptSCTP()
			if err != nil {
				select {
				case <-shutdownCtx.Done():
					logger.SctpLog.Info("shutting down listener")
					return
				default:
					switch err {
					case syscall.EINTR, syscall.EAGAIN:
						logger.SctpLog.Debugf("acceptSCTP: %+v", err)
					default:
						logger.SctpLog.Errorf("failed to accept: %+v", err)
					}
					continue
				}
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

			go handleConnection(newConn, readBufSize, handler)
		}
	}
}

func handleConnection(conn *sctp.SCTPConn, bufsize uint32, handler SCTPHandler) {
	wg.Add(1)
	defer wg.Done()
	buf := make([]byte, bufsize)

	defer func() {
		connections.Delete(conn)

		// if AMF call Stop(), then conn.Close() will return EBADF because conn has been closed inside Stop()
		if err := conn.Close(); err != nil && err != syscall.EBADF {
			logger.SctpLog.Errorf("close connection error: %+v", err)
		}
		logger.SctpLog.Infof("connection[addr: %+v] closed", conn.RemoteAddr())
	}()

	for {
		select {
		case <-shutdownCtx.Done():
			logger.SctpLog.Info("shutting down connection handler")
			return
		default:
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
}

func Stop() {
	logger.SctpLog.Info("initiating graceful shutdown")
	shutdownCancel()

	if sctpListener != nil {
		sctpListener.Close()
	}

	connections.Range(func(key, value any) bool {
		if conn, ok := key.(*sctp.SCTPConn); ok {
			conn.Close()
		}
		return true
	})

	wg.Wait()
	logger.SctpLog.Info("graceful shutdown completed")
}
