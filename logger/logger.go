// SPDX-FileCopyrightText: 2022 Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	logh         *zap.Logger
	CfgLog       *zap.SugaredLogger
	AppLog       *zap.SugaredLogger
	SctpLog      *zap.SugaredLogger
	GrpcLog      *zap.SugaredLogger
	DispatchLog  *zap.SugaredLogger
	DiscoveryLog *zap.SugaredLogger
	RanLog       *zap.SugaredLogger
)

const (
	FieldRanAddr string = "ran_addr"
)

func init() {
	config := zap.Config{
		Level:            zap.NewAtomicLevelAt(zap.InfoLevel),
		Development:      false,
		Encoding:         "console",
		EncoderConfig:    zap.NewProductionEncoderConfig(),
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
	}

	config.EncoderConfig.TimeKey = "timestamp"
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	config.EncoderConfig.MessageKey = "message"
	config.EncoderConfig.CallerKey = "caller"
	config.EncoderConfig.LevelKey = "level"
	config.EncoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder

	var err error
	logh, err = config.Build()
	if err != nil {
		panic(err)
	}

	CfgLog = logh.Sugar().With("component", "SCTP_LB", "category", "CFG")
	AppLog = logh.Sugar().With("component", "SCTP_LB", "category", "GRPC")
	SctpLog = logh.Sugar().With("component", "SCTP")
	GrpcLog = logh.Sugar().With("component", "Grpc")
	DispatchLog = logh.Sugar().With("component", "DISPATCH")
	DiscoveryLog = logh.Sugar().With("component", "discovery")
	RanLog = logh.Sugar().With("component", "RAN")
}

func SetLogLevel(level zapcore.Level) {
	logh = logh.WithOptions(zap.IncreaseLevel(level))
}

func SetReportCaller(set bool) {
	if set {
		logh = logh.WithOptions(zap.AddCaller())
	} else {
		logh = logh.WithOptions(zap.WithCaller(false))
	}
}
