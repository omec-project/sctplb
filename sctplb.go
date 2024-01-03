// SPDX-FileCopyrightText: 2022 Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"log"

	"github.com/omec-project/sctplb/backend"
	"github.com/omec-project/sctplb/config"
	"github.com/omec-project/sctplb/logger"
)

const (
	add_op = iota
	modify_op
	delete_op
)

func main() {
	logger.AppLog.Println("SCTP LB started")

	SimappConfig, err := config.InitConfigFactory("./config/sctplb.yaml")
	if err != nil {
		log.Fatalln(err)
	}

	// Read messages from SCTP Sockets and push it on channel
	logger.AppLog.Println("SCTP Port ", SimappConfig.Configuration.NgapPort, " grpc port : ", SimappConfig.Configuration.SctpGrpcPort)
	backend.ServiceRun(SimappConfig.Configuration.NgapIpList, SimappConfig.Configuration.NgapPort)

	var b = backend.BackendSvc{
		Cfg: SimappConfig,
	}
	b.DispatchAddServer()
}
