// SPDX-FileCopyrightText: 2022 Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/omec-project/sctplb/internal/backend"
	"github.com/omec-project/sctplb/internal/factory"
	"github.com/omec-project/sctplb/internal/logger"
	"github.com/urfave/cli/v3"
)

// shutdownTimeout bounds how long sctplb waits, after receiving SIGTERM, for
// in-flight gNB and backend NF connections to close before exiting anyway.
const shutdownTimeout = 10 * time.Second

type SCTPLB struct {
	cfg factory.Config
}

var sctplbCli = []cli.Flag{
	&cli.StringFlag{
		Name:     "cfg",
		Usage:    "sctplb config file",
		Required: true,
	},
}

func (*SCTPLB) GetCliCmd() (flags []cli.Flag) {
	return sctplbCli
}

func (s *SCTPLB) Initialize(_ context.Context, c *cli.Command) error {
	logger.AppLog.Infoln("sctp-lb started")
	absPath, err := filepath.Abs(c.String("cfg"))
	if err != nil {
		logger.CfgLog.Errorln(err)
		return err
	}

	s.cfg, err = factory.InitConfigFactory(absPath)
	if err != nil {
		logger.AppLog.Errorf("failed to initialize config: %v", err)
		return err
	}
	return nil
}

// Start runs the load balancer until it gets SIGTERM or SIGINT, then terminates it.
func (s *SCTPLB) Start(ctx context.Context) error {
	shutdownCtx, stop := signal.NotifyContext(ctx, syscall.SIGTERM, os.Interrupt)
	defer stop()

	// Read messages from SCTP Sockets and push it on channel
	logger.AppLog.Infof("sctp port: %d grpc port: %d", s.cfg.Configuration.NgapPort, s.cfg.Configuration.SctpGrpcPort)
	backend.ServiceRun(s.cfg.Configuration.NgapIpList, s.cfg.Configuration.NgapPort)

	b := backend.BackendSvc{
		Cfg: s.cfg,
	}
	b.DispatchAddServer(shutdownCtx)

	logger.AppLog.Infoln("received shutdown signal, shutting down sctplb")
	s.Terminate()
	return nil
}

// Terminate closes the SCTP and backend connections, waiting at most shutdownTimeout.
func (*SCTPLB) Terminate() {
	timeoutCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	backend.Shutdown(timeoutCtx)
	logger.AppLog.Infoln("sctplb shutdown complete")
}
