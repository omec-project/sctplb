// SPDX-FileCopyrightText: 2026 Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"testing"
)

func Test_SetN3iwfId(t *testing.T) {
	tests := []struct {
		name    string
		n3iwfId string
	}{
		{name: "set N3IWF-ID", n3iwfId: "42"},
		{name: "set N3IWF-ID zero value", n3iwfId: "0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ran := &Ran{}
			ran.SetN3iwfId(tt.n3iwfId)
			if ran.N3iwfId == nil {
				t.Errorf("SetN3iwfId() left N3iwfId nil, want %q", tt.n3iwfId)
			} else if *ran.N3iwfId != tt.n3iwfId {
				t.Errorf("SetN3iwfId() = %q, want %q", *ran.N3iwfId, tt.n3iwfId)
			}
		})
	}
}

func Test_RanID(t *testing.T) {
	gnbVal := "mcc001-mnc01-gnb1"
	n3iwfVal := "42"

	tests := []struct {
		name   string
		ran    *Ran
		wantID string
	}{
		{
			name:   "no ID set",
			ran:    &Ran{},
			wantID: "",
		},
		{
			name:   "gNB-ID set",
			ran:    &Ran{RanId: &gnbVal},
			wantID: "<Mcc:Mnc:GNbID mcc001-mnc01-gnb1>",
		},
		{
			name:   "N3IWF-ID set",
			ran:    &Ran{N3iwfId: &n3iwfVal},
			wantID: "<N3iwfID 42>",
		},
		{
			name:   "N3IWF-ID takes precedence when both set",
			ran:    &Ran{RanId: &gnbVal, N3iwfId: &n3iwfVal},
			wantID: "<N3iwfID 42>",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ran.RanID(); got != tt.wantID {
				t.Errorf("RanID() = %q, want %q", got, tt.wantID)
			}
		})
	}
}

func Test_RanFindByN3iwfId(t *testing.T) {
	ctx := &SctplbContext{}

	r1 := &Ran{GnbIp: "10.0.0.1"}
	r1.SetN3iwfId("100")
	r2 := &Ran{GnbIp: "10.0.0.2"}
	r2.SetN3iwfId("200")
	r3 := &Ran{GnbIp: "10.0.0.3"} // gNB only, no N3iwfId

	ctx.RanPool.Store("key1", r1)
	ctx.RanPool.Store("key2", r2)
	ctx.RanPool.Store("key3", r3)

	tests := []struct {
		name      string
		n3iwfId   string
		wantGnbIp string
		wantOk    bool
	}{
		{name: "find r1", n3iwfId: "100", wantGnbIp: "10.0.0.1", wantOk: true},
		{name: "find r2", n3iwfId: "200", wantGnbIp: "10.0.0.2", wantOk: true},
		{name: "not found", n3iwfId: "999", wantGnbIp: "", wantOk: false},
		{name: "gNB-only RAN not returned", n3iwfId: "", wantGnbIp: "", wantOk: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ran, ok := ctx.RanFindByN3iwfId(tt.n3iwfId)
			if ok != tt.wantOk {
				t.Errorf("RanFindByN3iwfId(%q) ok = %v, want %v", tt.n3iwfId, ok, tt.wantOk)
			}
			if ok && ran.GnbIp != tt.wantGnbIp {
				t.Errorf("RanFindByN3iwfId(%q) GnbIp = %q, want %q", tt.n3iwfId, ran.GnbIp, tt.wantGnbIp)
			}
		})
	}
}
