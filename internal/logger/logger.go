// SPDX-FileCopyrightText: 2022 Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

package logger

import (
	utilLogger "github.com/omec-project/util/logger"
	"go.uber.org/zap/zapcore"
)

const (
	FieldRanAddr string = "ran_addr"
)

var component = utilLogger.NewComponent("SCTP_LB")

var (
	CfgLog       = component.Category("CFG")
	AppLog       = component.Category("App")
	SctpLog      = component.Category("SCTP")
	GrpcLog      = component.Category("Grpc")
	DispatchLog  = component.Category("DISPATCH")
	DiscoveryLog = component.Category("discovery")
	RanLog       = component.Category("RAN")
)

// SetLogLevel: set the log level (panic|fatal|error|warn|info|debug)
func SetLogLevel(level zapcore.Level) {
	CfgLog.Infoln("set log level:", level)
	component.SetLevel(level)
}
