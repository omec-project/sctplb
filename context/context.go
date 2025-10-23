// SPDX-FileCopyrightText: 2022-present Intel Corporation
// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package context

import (
	"net"
	"strings"
	"sync"

	"github.com/omec-project/sctplb/logger"
	"go.uber.org/zap"
)

var sctplbContext = SctplbContext{}

type SctplbContext struct {
	RanPool  sync.Map // map[net.Conn]*Ran
	Backends []NF
}

var (
	nfNum int
	mutex sync.Mutex
)

type Ran struct {
	RanId *string
	Name  string
	GnbIp string
	/* socket Connect*/
	Conn net.Conn `json:"-"`

	Log *zap.SugaredLogger `json:"-"`
}

func (ran *Ran) Remove() {
	ran.Log.Infof("remove RAN context[ID: %+v]", ran.RanID())
	sctplbContext.DeleteRan(ran.Conn)
}

func (ran *Ran) SetRanId(gnbId string) {
	ran.RanId = &gnbId
}

func (ran *Ran) RanID() string {
	if ran.RanId != nil {
		var builder strings.Builder
		builder.WriteString("<Mcc:Mnc:GNbID ")
		builder.WriteString(*ran.RanId)
		builder.WriteString(">")
		return builder.String()
	}
	return ""
}

func (context *SctplbContext) NewRan(conn net.Conn) *Ran {
	ran := Ran{}
	ran.Conn = conn
	ran.GnbIp = conn.RemoteAddr().String()
	ran.Log = logger.RanLog.Desugar().Sugar().With(logger.FieldRanAddr, conn.RemoteAddr().String())
	context.RanPool.Store(conn, &ran)
	return &ran
}

// use net.Conn to find RAN context, return *Ran and ok bit
func (context *SctplbContext) RanFindByConn(conn net.Conn) (*Ran, bool) {
	if value, ok := context.RanPool.Load(conn); ok {
		return value.(*Ran), ok
	}
	return nil, false
}

// get Ran using RanId
func (context *SctplbContext) RanFindByGnbId(gnbId string) (ran *Ran, ok bool) {
	context.RanPool.Range(func(key, value any) bool {
		candidate := value.(*Ran)
		if ok = (*candidate.RanId == gnbId); ok {
			ran = candidate
			return false
		}
		return true
	})
	return
}

// get Ran using GnbIp
func (context *SctplbContext) RanFindByGnbIp(gnbIp string) (ran *Ran, ok bool) {
	context.RanPool.Range(func(key, value any) bool {
		candidate := value.(*Ran)
		if ok = (candidate.GnbIp == gnbIp); ok {
			ran = candidate
			return false
		}
		return true
	})
	return
}

func (context *SctplbContext) DeleteRan(conn net.Conn) {
	context.RanPool.Delete(conn)
}

// Create new AMF context
func Sctplb_Self() *SctplbContext {
	return &sctplbContext
}

type NF interface {
	ConnectToServer(int)
	Send([]byte, bool, *Ran) error
	State() bool
}

func (context *SctplbContext) DeleteNF(target NF) {
	for i, instance := range sctplbContext.Backends {
		if instance == target {
			sctplbContext.Backends[i] = sctplbContext.Backends[len(sctplbContext.Backends)-1]
			sctplbContext.Backends = sctplbContext.Backends[:len(sctplbContext.Backends)-1]
			nfNum--
			break
		}
	}
}

func (context *SctplbContext) Iterate(handler func(k int, v NF)) {
	for k, v := range context.Backends {
		handler(k, v)
	}
}

func (context *SctplbContext) AddNF(target NF) {
	context.Backends = append(context.Backends, target)
	nfNum++
}

func (context *SctplbContext) NFLength() int {
	return nfNum
}

func (context *SctplbContext) Lock() {
	mutex.Lock()
}

func (context *SctplbContext) Unlock() {
	mutex.Unlock()
}
