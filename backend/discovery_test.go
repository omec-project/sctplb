// SPDX-FileCopyrightText: 2026 Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

package backend

import (
	stdctx "context"
	"errors"
	"net"
	"testing"

	"github.com/omec-project/sctplb/config"
	"github.com/omec-project/sctplb/context"
)

// fakeDiscovery replaces the resolver and the backend start for one test and
// counts what discovery does.
type fakeDiscovery struct {
	err     error
	ips     []net.IPAddr
	started []string
	lookups int
}

func useFakeDiscovery(t *testing.T, f *fakeDiscovery) {
	t.Helper()
	context.Sctplb_Self().Reset()
	oldLookup, oldStart := lookupIPAddr, startBackend
	lookupIPAddr = func(stdctx.Context, string) ([]net.IPAddr, error) {
		f.lookups++
		return f.ips, f.err
	}
	startBackend = func(backend context.NF, _ int) {
		f.started = append(f.started, backend.(*GrpcServer).address)
	}
	t.Cleanup(func() {
		lookupIPAddr, startBackend = oldLookup, oldStart
		context.Sctplb_Self().Reset()
	})
}

func grpcService(backendType string) BackendSvc {
	var cfg config.Config
	cfg.Configuration = &config.Configuration{Type: backendType, SctpGrpcPort: 9000}
	return BackendSvc{Cfg: cfg}
}

// One pass resolves the name once and returns: the loop used to resolve it again
// at once after every successful lookup.
func TestDiscoverService_ResolvesOnceAndAddsNewIPv4Backends(t *testing.T) {
	f := &fakeDiscovery{ips: []net.IPAddr{
		{IP: net.ParseIP("10.0.0.1")},
		{IP: net.ParseIP("10.0.0.2")},
		{IP: net.ParseIP("fd00::1")},
	}}
	useFakeDiscovery(t, f)
	svc := grpcService("grpc")

	svc.discoverService("amf")
	if f.lookups != 1 {
		t.Fatalf("lookups = %d, want 1", f.lookups)
	}
	if got := context.Sctplb_Self().NFLength(); got != 2 {
		t.Fatalf("backends = %d, want 2 (IPv6 is not used)", got)
	}

	// A second pass finds the same addresses and adds nothing.
	svc.discoverService("amf")
	if f.lookups != 2 || context.Sctplb_Self().NFLength() != 2 || len(f.started) != 2 {
		t.Fatalf("after a second pass: lookups %d, backends %d, started %v; want 2, 2, 2",
			f.lookups, context.Sctplb_Self().NFLength(), f.started)
	}
}

func TestDiscoverService_FailedLookupAddsNothing(t *testing.T) {
	f := &fakeDiscovery{err: errors.New("no such host")}
	useFakeDiscovery(t, f)

	grpcService("grpc").discoverService("amf")
	if f.lookups != 1 || context.Sctplb_Self().NFLength() != 0 {
		t.Fatalf("lookups %d, backends %d; want 1, 0", f.lookups, context.Sctplb_Self().NFLength())
	}
}

// An unsupported backend type used to add a nil backend and call it.
func TestDiscoverService_UnsupportedTypeAddsNothing(t *testing.T) {
	f := &fakeDiscovery{ips: []net.IPAddr{{IP: net.ParseIP("10.0.0.1")}}}
	useFakeDiscovery(t, f)

	grpcService("http").discoverService("amf")
	if context.Sctplb_Self().NFLength() != 0 || len(f.started) != 0 {
		t.Fatalf("backends %d, started %v; want none", context.Sctplb_Self().NFLength(), f.started)
	}
}
