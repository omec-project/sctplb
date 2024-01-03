// SPDX-FileCopyrightText: 2023 Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"testing"

	"github.com/omec-project/sctplb/context"
	"github.com/stretchr/testify/require"
)

func initBackendNF() {
	ctx := context.Sctplb_Self()
	nfList := []*GrpcServer{
		{
			address: "127.0.0.1",
		},
		{
			address: "127.0.0.2",
		},
		{
			address: "127.0.0.3",
		},
		{
			address: "127.0.0.4",
		},
		{
			address: "127.0.0.5",
		},
	}
	for _, n := range nfList {
		ctx.AddNF(n)
	}
}

func Test_RoundRobin(t *testing.T) {
	ctx := context.Sctplb_Self()
	initBackendNF()

	tests := []struct {
		name string
		want *GrpcServer
	}{
		{
			name: "Get BackendNF - 1",
			want: ctx.Backends[0].(*GrpcServer),
		},
		{
			name: "Get BackendNF - 2",
			want: ctx.Backends[1].(*GrpcServer),
		},
		{
			name: "Get BackendNF - 3",
			want: ctx.Backends[2].(*GrpcServer),
		},
		{
			name: "Get BackendNF - 4",
			want: ctx.Backends[3].(*GrpcServer),
		},
		{
			name: "Get BackendNF - 5",
			want: ctx.Backends[4].(*GrpcServer),
		},
		{
			name: "Get BackendNF - 6",
			want: ctx.Backends[0].(*GrpcServer),
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				instance := RoundRobin()
				require.Equal(t, instance.(*GrpcServer), tt.want)
			},
		)
	}
}

func Test_Iterate(t *testing.T) {
	ctx := context.Sctplb_Self()
	initBackendNF()

	tests := []struct {
		name string
		want []context.NF
	}{
		{
			name: "Test Iterate",
			want: ctx.Backends,
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				var count int
				ctx.Iterate(func(k int, v context.NF) {
					require.Equal(t, tt.want[k], v)
					count++
				})
				require.Equal(t, ctx.NFLength(), count)
			},
		)
	}
}
