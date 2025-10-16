// SPDX-FileCopyrightText: 2023 Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"reflect"
	"testing"

	"github.com/omec-project/sctplb/context"
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
				got := instance.(*GrpcServer)
				if got.address != tt.want.address {
					t.Errorf("RoundRobin() address mismatch. got = %q, want = %q", got.address, tt.want.address)
				}

				if got.state != tt.want.state {
					t.Errorf("RoundRobin() state mismatch. got = %v, want = %v", got.state, tt.want.state)
				}

				// For conn, gc, stream - check if they're both nil or both non-nil
				if (got.conn == nil) != (tt.want.conn == nil) {
					t.Errorf("RoundRobin() conn nil mismatch. got nil = %v, want nil = %v", got.conn == nil, tt.want.conn == nil)
				}

				if (got.gc == nil) != (tt.want.gc == nil) {
					t.Errorf("RoundRobin() gc nil mismatch. got nil = %v, want nil = %v", got.gc == nil, tt.want.gc == nil)
				}

				if (got.stream == nil) != (tt.want.stream == nil) {
					t.Errorf("RoundRobin() stream nil mismatch. got nil = %v, want nil = %v", got.stream == nil, tt.want.stream == nil)
				}
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
					if !reflect.DeepEqual(v, tt.want[k]) {
						t.Errorf("NF at index %d mismatch. got = %+v, want = %+v", k, v, tt.want[k])
					}
					count++
				})

				if ctx.NFLength() != count {
					t.Errorf("NFLength mismatch. got = %d, want = %d", ctx.NFLength(), count)
				}
			},
		)
	}
}
