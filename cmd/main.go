// SPDX-FileCopyrightText: 2022 Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"os"

	"github.com/omec-project/sctplb/internal/logger"
	"github.com/omec-project/sctplb/internal/service"
	"github.com/urfave/cli/v3"
)

var SCTPLB = &service.SCTPLB{}

func main() {
	app := &cli.Command{}
	app.Name = "sctplb"
	logger.AppLog.Infoln(app.Name)
	app.Usage = "SCTP Load Balancer"
	app.UsageText = "sctplb -cfg <sctplb_config_file.conf>"
	app.Action = action
	app.Flags = SCTPLB.GetCliCmd()
	if err := app.Run(context.Background(), os.Args); err != nil {
		logger.AppLog.Fatalf("SCTPLB run error: %v", err)
	}
}

func action(ctx context.Context, c *cli.Command) error {
	if err := SCTPLB.Initialize(ctx, c); err != nil {
		return err
	}
	return SCTPLB.Start(ctx)
}
