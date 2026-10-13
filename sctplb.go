// SPDX-FileCopyrightText: 2022 Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/omec-project/sctplb/backend"
	"github.com/omec-project/sctplb/config"
	"github.com/omec-project/sctplb/logger"
	"github.com/urfave/cli/v3"
)

// shutdownTimeout bounds how long sctplb waits, after receiving SIGTERM, for
// in-flight gNB and backend NF connections to close before exiting anyway.
const shutdownTimeout = 10 * time.Second

func main() {
	app := &cli.Command{}
	app.Name = "sctplb"
	logger.AppLog.Infoln(app.Name)
	app.Usage = "SCTP Load Balancer"
	app.UsageText = "sctplb -cfg <sctplb_config_file.conf>"
	app.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:     "cfg",
			Usage:    "sctplb config file",
			Required: true,
		},
	}
	app.Action = action
	if err := app.Run(context.Background(), os.Args); err != nil {
		logger.AppLog.Fatalf("SCTPLB run error: %v", err)
	}
}

func action(ctx context.Context, c *cli.Command) error {
	logger.AppLog.Infoln("sctp-lb started")
	cfg := c.String("cfg")
	absPath, err := filepath.Abs(cfg)
	if err != nil {
		logger.CfgLog.Errorln(err)
		return err
	}

	sctplbConfig, err := config.InitConfigFactory(absPath)
	if err != nil {
		logger.AppLog.Errorf("failed to initialize config: %v", err)
		return err
	}

	shutdownCtx, stop := signal.NotifyContext(ctx, syscall.SIGTERM, os.Interrupt)
	defer stop()

	// Read messages from SCTP Sockets and push it on channel
	logger.AppLog.Infof("sctp port: %d grpc port: %d", sctplbConfig.Configuration.NgapPort, sctplbConfig.Configuration.SctpGrpcPort)
	backend.ServiceRun(sctplbConfig.Configuration.NgapIpList, sctplbConfig.Configuration.NgapPort)

	b := backend.BackendSvc{
		Cfg: sctplbConfig,
	}
	b.DispatchAddServer(shutdownCtx)

	logger.AppLog.Infoln("received shutdown signal, shutting down sctplb")
	timeoutCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	backend.Shutdown(timeoutCtx)
	logger.AppLog.Infoln("sctplb shutdown complete")

	return nil
}
