// SPDX-FileCopyrightText: 2023 Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_InitConfigFactory(t *testing.T) {
	xcfg := Config{
		Info: &Info{
			Description: "SctpLb initial local configuration",
			Version:     "1.0.1",
		},
		Logger: &Logger{},
		Configuration: &Configuration{
			Type: "grpc",
			Services: []Service{
				{
					Uri: "sctplb",
				},
			},
			NgapIpList:   []string{"0.0.0.0"},
			NgapPort:     38416,
			SctpGrpcPort: 5000,
		},
	}

	t.Run("", func(t *testing.T) {
		cfg, err := InitConfigFactory("./sctplb.yaml")
		require.NoError(t, err)
		require.Equal(t, xcfg, cfg)
	},
	)
}
