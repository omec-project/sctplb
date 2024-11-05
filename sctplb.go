// SPDX-FileCopyrightText: 2022 Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"

	"github.com/omec-project/sctplb/backend"
	"github.com/omec-project/sctplb/config"
	"github.com/omec-project/sctplb/logger"
	"github.com/urfave/cli/v2"
)

func main() {
	app := cli.NewApp()
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
	if err := app.Run(os.Args); err != nil {
		logger.AppLog.Fatalf("SCTPLB run error: %v", err)
	}
}

func action(c *cli.Context) error {
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

	// Read messages from SCTP Sockets and push it on channel
	logger.AppLog.Infof("sctp port: %d grpc port: %d", sctplbConfig.Configuration.NgapPort, sctplbConfig.Configuration.SctpGrpcPort)
	backend.ServiceRun(sctplbConfig.Configuration.NgapIpList, sctplbConfig.Configuration.NgapPort)

	b := backend.BackendSvc{
		Cfg: sctplbConfig,
	}
	b.DispatchAddServer()

	return nil
}
