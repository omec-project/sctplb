---
title: "feat: Add N3IWF-ID support alongside GNB-ID in sctplb"
type: feat
status: completed
date: 2026-03-19
issue: "https://github.com/omec-project/sctplb/issues/225"
---

# feat: Add N3IWF-ID Support Alongside GNB-ID in sctplb

## Overview

sctplb currently identifies RAN connections exclusively using `GnbId` (gNodeB ID). The N3IWF (Non-3GPP Interworking Function) also connects to the AMF over N2/NGAP via SCTP, but uses a distinct N3IWF-ID identifier. This feature adds first-class N3IWF-ID support to sctplb so it can proxy NGAP traffic for both gNBs and N3IWF nodes.

This is a cross-repo change: `sctplb` owns the protobuf definition (`client.proto`) consumed by `github.com/omec-project/amf`.

---

## Problem Statement / Motivation

sctplb acts as a transparent SCTP load balancer between RAN nodes and AMF instances. It doesn't parse NGAP — it relies on the AMF to send back RAN identifiers (via `AmfMessage`) so it can route responses to the correct SCTP connection.

Today the wire format (`client.proto`) only carries `GnbId` and `GnbIpAddr`. An N3IWF node:
- Connects to sctplb via SCTP (same as gNB)
- Sends NGSetupRequest over NGAP (same as gNB)
- Is assigned an N3IWF-ID by the AMF (different namespace from GNB-ID)

Without a dedicated `N3iwfId` field in the proto, the AMF cannot communicate back to sctplb which SCTP connection to use for N3IWF responses, causing message routing failures for non-3GPP access.

---

## Proposed Solution

Add an `N3iwfId` field to both `SctplbMessage` and `AmfMessage` in `client.proto`, add N3IWF-specific `msgType` enum values, and update the sctplb runtime to populate and route using `N3iwfId` in parallel to the existing `GnbId` path.

The `Ran` struct gains an `N3iwfId *string` field alongside the existing `RanId *string` (which represents gNB-ID). The AMF-side response is the signal: if `AmfMessage.N3iwfId` is set (and `GnbId` is empty), sctplb treats the connection as an N3IWF.

---

## Technical Considerations

### Protocol (proto3 backward compatibility)
Adding new fields to a proto3 message is fully backward-compatible. Old AMF deployments will send zero-value (empty string) for the new `N3iwfId` field; new sctplb code treats empty `N3iwfId` as absent. No existing gNB flows are affected.

### Identity model
sctplb learns a RAN's identity lazily — only after AMF processes the NGSetupRequest and sends back a `AmfMessage` with `GnbIpAddr` + `GnbId` (or the new `N3iwfId`). The distinction between gNB and N3IWF is never explicit at SCTP connect time; it's communicated by the AMF. This means no NGAP parsing is needed.

### Ran struct design
Add `N3iwfId *string` as a parallel field to `RanId *string`. This mirrors the proto's separate fields and avoids a discriminator enum that would add complexity. `RanID()` branches on which field is non-nil.

### Redirect handling
`readFromServer()` handles `REDIRECT_MSG` by forwarding messages to another AMF backend. The forwarded `SctplbMessage` must carry the correct identifier (either `GnbId` or `N3iwfId`) from the incoming `AmfMessage`.

### INIT messages on new AMF connection
When a new AMF instance connects, `ConnectToServer()` iterates `RanPool` and sends `INIT_MSG` for each known RAN. This loop must also send `N3iwfId` for N3IWF RANs (currently it silently skips RANs without a `RanId`).

---

## Acceptance Criteria

- [x] `client.proto` adds `N3iwfId string` at field 7 in `SctplbMessage` and field 8 in `AmfMessage`
- [x] `client.proto` adds `N3IWF_MSG=7`, `N3IWF_DISC=8`, `N3IWF_CONN=9` to the `msgType` enum
- [x] `sdcoreAmfServer/client.pb.go` and `client_grpc.pb.go` are regenerated and SPDX headers restored
- [x] `context/context.go`: `Ran` struct has `N3iwfId *string` field
- [x] `context/context.go`: `SetN3iwfId(id string)` method added to `Ran`
- [x] `context/context.go`: `RanFindByN3iwfId(id string)` method added to `SctplbContext`
- [x] `context/context.go`: `RanID()` formats N3IWF identity as `<N3iwfID <value>>` when `N3iwfId` is set
- [x] `backend/grpc.go` `Send()`: uses `N3IWF_MSG`/`N3IWF_DISC` and populates `N3iwfId` for N3IWF RANs
- [x] `backend/grpc.go` `readFromServer()`: routes responses by `N3iwfId` when `GnbId` is empty
- [x] `backend/grpc.go` `readFromServer()`: REDIRECT_MSG propagates `N3iwfId` when present
- [x] `backend/grpc.go` `ConnectToServer()`: INIT loop sends `N3iwfId` for N3IWF RANs
- [x] All existing gNB tests continue to pass unchanged
- [x] New unit tests cover the `RanFindByN3iwfId` and `SetN3iwfId` paths
- [ ] AMF repo (`github.com/omec-project/amf`) updated in a coordinated PR to populate `N3iwfId` in `AmfMessage` responses for N3IWF NGSetup flows

---

## Dependencies & Risks

| Item | Notes |
|------|-------|
| **AMF repo coordination** | Proto changes in sctplb must be mirrored in AMF. Both sides need to deploy together (or sctplb deployed first, since new fields are zero-value on old AMF). |
| **protoc toolchain** | No `proto-gen` Makefile target exists. Regeneration requires `protoc` + `protoc-gen-go` + `protoc-gen-go-grpc` matching the versions in the generated file headers (`protoc v5.28.2`, `protoc-gen-go v1.35.1`, `protoc-gen-go-grpc v1.2.0`). SPDX headers must be manually restored after regeneration. |
| **N3IWF_CONN usage** | `GNB_CONN=6` is defined but unused in current code. `N3IWF_CONN=9` should follow the same pattern (define now, use later). |
| **No CLAUDE.md** | No repo-level conventions file. Follow existing code style: table-driven tests, `t.Errorf` for non-fatal assertions, SPDX headers on all new files. |

---

## Implementation Sketch

### `client.proto`

```proto
enum msgType {
    UNKNOWN      = 0;
    INIT_MSG     = 1;
    GNB_MSG      = 2;
    AMF_MSG      = 3;
    REDIRECT_MSG = 4;
    GNB_DISC     = 5;
    GNB_CONN     = 6;
    N3IWF_MSG    = 7;  // new
    N3IWF_DISC   = 8;  // new
    N3IWF_CONN   = 9;  // new
}

message SctplbMessage {
    string SctplbId  = 1;
    msgType Msgtype  = 2;
    string GnbIpAddr = 3;
    string VerboseMsg = 4;
    bytes Msg        = 5;
    string GnbId     = 6;
    string N3iwfId   = 7;  // new
}

message AmfMessage {
    string AmfId     = 1;
    string RedirectId = 2;
    msgType Msgtype  = 3;
    string GnbIpAddr = 4;
    string GnbId     = 5;
    string VerboseMsg = 6;
    bytes Msg        = 7;
    string N3iwfId   = 8;  // new
}
```

### `context/context.go` — `Ran` struct additions

```go
// context/context.go (Ran struct)
type Ran struct {
    RanId   *string   // gNB-ID
    N3iwfId *string   // N3IWF-ID (new)
    Name    string
    GnbIp   string
    Conn    net.Conn `json:"-"`
    Log     *zap.SugaredLogger `json:"-"`
}

func (ran *Ran) SetN3iwfId(id string) {
    ran.N3iwfId = &id
}

func (ran *Ran) RanID() string {
    if ran.N3iwfId != nil {
        return "<N3iwfID " + *ran.N3iwfId + ">"
    }
    if ran.RanId != nil {
        return "<Mcc:Mnc:GNbID " + *ran.RanId + ">"
    }
    return ""
}
```

### `context/context.go` — new lookup

```go
// context/context.go
func (context *SctplbContext) RanFindByN3iwfId(n3iwfId string) (ran *Ran, ok bool) {
    context.RanPool.Range(func(key, value any) bool {
        candidate := value.(*Ran)
        if candidate.N3iwfId != nil {
            if ok = (*candidate.N3iwfId == n3iwfId); ok {
                ran = candidate
                return false
            }
        }
        return true
    })
    return
}
```

### `backend/grpc.go` — `readFromServer()` routing (updated else-branch)

```go
// backend/grpc.go: readFromServer() — the non-INIT, non-REDIRECT branch
var ran *context.Ran
switch {
case response.GnbIpAddr != "" && response.GnbId != "":
    // NGSetupResponse for gNB: bind GnbId to the IP-keyed Ran
    ran, _ = context.Sctplb_Self().RanFindByGnbIp(response.GnbIpAddr)
    if ran != nil {
        ran.SetRanId(response.GnbId)
    }
case response.GnbIpAddr != "" && response.N3iwfId != "":
    // NGSetupResponse for N3IWF: bind N3iwfId to the IP-keyed Ran
    ran, _ = context.Sctplb_Self().RanFindByGnbIp(response.GnbIpAddr)
    if ran != nil {
        ran.SetN3iwfId(response.N3iwfId)
    }
case response.GnbId != "":
    ran, _ = context.Sctplb_Self().RanFindByGnbId(response.GnbId)
case response.N3iwfId != "":
    ran, _ = context.Sctplb_Self().RanFindByN3iwfId(response.N3iwfId)
default:
    logger.RanLog.Infoln("received message with no RAN identifier from backend NF")
}
```

### `backend/grpc.go` — `Send()` (N3IWF branch)

```go
// backend/grpc.go: Send()
} else {
    if ran.N3iwfId != nil {
        t.Msgtype = gClient.MsgType_N3IWF_MSG
        t.VerboseMsg = "Hello From N3IWF Message !"
        t.N3iwfId = *ran.N3iwfId
    } else if ran.RanId != nil {
        t.Msgtype = gClient.MsgType_GNB_MSG
        t.VerboseMsg = "Hello From gNB Message !"
        t.GnbId = *ran.RanId
    } else {
        t.Msgtype = gClient.MsgType_GNB_MSG
        t.VerboseMsg = "Hello From gNB Message !"
        t.GnbIpAddr = ran.Conn.RemoteAddr().String()
    }
```

---

## References & Research

### Internal References

- Proto schema: `client.proto:8-39`
- `Ran` struct: `context/context.go:31-39`
- `RanFindByGnbId`: `context/context.go:79-88`
- `Send()`: `backend/grpc.go:160-184`
- `readFromServer()` routing logic: `backend/grpc.go:119-141`
- `ConnectToServer()` INIT loop: `backend/grpc.go:44-78`
- `REDIRECT_MSG` forwarding: `backend/grpc.go:90-113`
- Test helper pattern: `backend/sched_test.go`
- Proto generation versions: `sdcoreAmfServer/client.pb.go:6-9`

### Related Work

- Issue #225: https://github.com/omec-project/sctplb/issues/225
- AMF protos: https://github.com/omec-project/amf/tree/main/protos
