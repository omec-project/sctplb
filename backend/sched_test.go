package backend

import (
	"testing"

	"github.com/omec-project/sctplb/context"
	"github.com/stretchr/testify/require"
)

func initBackendNF() {
	ctx := context.Sctplb_Self()
	nfList := []*BackendNF{
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
		want *BackendNF
	}{
		{
			name: "Get BackendNF - 1",
			want: ctx.Backends[0].(*BackendNF),
		},
		{
			name: "Get BackendNF - 2",
			want: ctx.Backends[1].(*BackendNF),
		},
		{
			name: "Get BackendNF - 3",
			want: ctx.Backends[2].(*BackendNF),
		},
		{
			name: "Get BackendNF - 4",
			want: ctx.Backends[3].(*BackendNF),
		},
		{
			name: "Get BackendNF - 5",
			want: ctx.Backends[4].(*BackendNF),
		},
		{
			name: "Get BackendNF - 6",
			want: ctx.Backends[0].(*BackendNF),
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				instance := RoundRobin()
				require.Equal(t, instance.(*BackendNF), tt.want)
			},
		)
	}
}

func Test_Iterate(t *testing.T) {
	ctx := context.Sctplb_Self()
	initBackendNF()

	tests := []struct {
		name string
		want []interface{}
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
				ctx.Iterate(func(k int, v interface{}) {
					require.Equal(t, tt.want[k], v)
					count++
				})
				require.Equal(t, ctx.NFLength(), count)
			},
		)
	}
}
