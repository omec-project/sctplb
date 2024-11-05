// SPDX-FileCopyrightText: 2022 Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	log          *zap.Logger
	CfgLog       *zap.SugaredLogger
	AppLog       *zap.SugaredLogger
	SctpLog      *zap.SugaredLogger
	GrpcLog      *zap.SugaredLogger
	DispatchLog  *zap.SugaredLogger
	DiscoveryLog *zap.SugaredLogger
	RanLog       *zap.SugaredLogger
	atomicLevel  zap.AtomicLevel
)

const (
	FieldRanAddr string = "ran_addr"
)

func init() {
	atomicLevel = zap.NewAtomicLevelAt(zap.InfoLevel)
	config := zap.Config{
		Level:            atomicLevel,
		Development:      false,
		Encoding:         "console",
		EncoderConfig:    zap.NewProductionEncoderConfig(),
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
	}

	config.EncoderConfig.TimeKey = "timestamp"
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	config.EncoderConfig.LevelKey = "level"
	config.EncoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	config.EncoderConfig.CallerKey = "caller"
	config.EncoderConfig.EncodeCaller = zapcore.ShortCallerEncoder
	config.EncoderConfig.MessageKey = "message"
	config.EncoderConfig.StacktraceKey = ""

	var err error
	log, err = config.Build()
	if err != nil {
		panic(err)
	}

	CfgLog = log.Sugar().With("component", "SCTP_LB", "category", "CFG")
	AppLog = log.Sugar().With("component", "SCTP_LB", "category", "App")
	SctpLog = log.Sugar().With("component", "SCTP_LB", "category", "SCTP")
	GrpcLog = log.Sugar().With("component", "SCTP_LB", "category", "Grpc")
	DispatchLog = log.Sugar().With("component", "SCTP_LB", "category", "DISPATCH")
	DiscoveryLog = log.Sugar().With("component", "SCTP_LB", "category", "discovery")
	RanLog = log.Sugar().With("component", "SCTP_LB", "category", "RAN")
}

func GetLogger() *zap.Logger {
	return log
}

// SetLogLevel: set the log level (panic|fatal|error|warn|info|debug)
func SetLogLevel(level zapcore.Level) {
	CfgLog.Infoln("set log level:", level)
	atomicLevel.SetLevel(level)
}
