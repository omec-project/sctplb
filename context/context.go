// SPDX-FileCopyrightText: 2022-present Intel Corporation
// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package context

import (
	"fmt"
	"net"
	"sync"

	"github.com/omec-project/sctplb/logger"
	"github.com/sirupsen/logrus"
)

var sctplbContext = SctplbContext{}

type SctplbContext struct {
	RanPool  sync.Map // map[net.Conn]*Ran
	Backends []interface{}
}

var (
	nfNum int
	next  int
	mutex sync.Mutex
)

type Ran struct {
	RanId *string
	Name  string
	GnbIp string
	/* socket Connect*/
	Conn net.Conn `json:"-"`

	Log *logrus.Entry `json:"-"`
}

func (ran *Ran) Remove() {
	ran.Log.Infof("Remove RAN Context[ID: %+v]", ran.RanID())
	sctplbContext.DeleteRan(ran.Conn)
}

func (ran *Ran) SetRanId(gnbId string) {
	ran.RanId = &gnbId
}

func (ran *Ran) RanID() string {
	if ran.RanId != nil {
		return fmt.Sprintf("<Mcc:Mnc:GNbID %s>", ran.RanId)
	}
	return ""
}

func (context *SctplbContext) NewRan(conn net.Conn) *Ran {
	ran := Ran{}
	ran.Conn = conn
	ran.GnbIp = conn.RemoteAddr().String()
	ran.Log = logger.RanLog.WithField(logger.FieldRanAddr, conn.RemoteAddr().String())
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
	context.RanPool.Range(func(key, value interface{}) bool {
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
	context.RanPool.Range(func(key, value interface{}) bool {
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

func (context *SctplbContext) DeleteNF(target interface{}) {
	for i, instance := range sctplbContext.Backends {
		if instance == target {
			sctplbContext.Backends[i] = sctplbContext.Backends[len(sctplbContext.Backends)-1]
			sctplbContext.Backends = sctplbContext.Backends[:len(sctplbContext.Backends)-1]
			nfNum--
			break
		}
	}
}

func (context *SctplbContext) Iterate(handler func(k int, v interface{})) {
	mutex.Lock()
	for k, v := range context.Backends {
		handler(k, v)
	}
	mutex.Unlock()
}

func (context *SctplbContext) AddNF(target interface{}) {
	mutex.Lock()
	context.Backends = append(context.Backends, target)
	nfNum++
	mutex.Unlock()
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
