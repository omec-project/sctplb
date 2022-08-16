// SPDX-FileCopyrightText: 2022 Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

package logger

import (
	"time"

	formatter "github.com/antonfisher/nested-logrus-formatter"
	"github.com/sirupsen/logrus"
)

var (
	logh         *logrus.Logger
	CfgLog       *logrus.Entry
	AppLog       *logrus.Entry
	SctpLog      *logrus.Entry
	DispatchLog  *logrus.Entry
	DiscoveryLog *logrus.Entry
	RanLog       *logrus.Entry
)

const (
	FieldRanAddr string = "ran_addr"
)

func init() {
	logh = logrus.New()
	logh.SetReportCaller(false)

	logh.Formatter = &formatter.Formatter{
		TimestampFormat: time.RFC3339,
		TrimMessages:    true,
		NoFieldsSpace:   true,
		HideKeys:        true,
		FieldsOrder:     []string{"component", "category", FieldRanAddr},
	}

	CfgLog = logh.WithFields(logrus.Fields{"component": "SCTP_LB", "category": "CFG"})
	AppLog = logh.WithFields(logrus.Fields{"component": "SCTP_LB", "category": "GRPC"})
	SctpLog = logh.WithFields(logrus.Fields{"component": "SCTP"})
	DispatchLog = logh.WithFields(logrus.Fields{"component": "DISPATCH"})
	DiscoveryLog = logh.WithFields(logrus.Fields{"component": "discovery"})
	RanLog = logh.WithFields(logrus.Fields{"component": "RAN"})
}

func SetLogLevel(level logrus.Level) {
	logh.SetLevel(level)
}

func SetReportCaller(set bool) {
	logh.SetReportCaller(set)
}
