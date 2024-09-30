// SPDX-FileCopyrightText: 2022 Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"github.com/omec-project/sctplb/backend"
	"github.com/omec-project/sctplb/config"
	"github.com/omec-project/sctplb/logger"
)

func main() {
	logger.AppLog.Infoln("sctp-lb started")

	SimappConfig, err := config.InitConfigFactory("./config/sctplb.yaml")
	if err != nil {
		logger.AppLog.Fatalln("failed to initialize config", "error", err)
	}

	// Read messages from SCTP Sockets and push it on channel
	logger.AppLog.Infof("sctp port: %d grpc port: %d", SimappConfig.Configuration.NgapPort, SimappConfig.Configuration.SctpGrpcPort)
	backend.ServiceRun(SimappConfig.Configuration.NgapIpList, SimappConfig.Configuration.NgapPort)

	b := backend.BackendSvc{
		Cfg: SimappConfig,
	}
	b.DispatchAddServer()
}
