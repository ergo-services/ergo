// Copyright 2012-2013 Metachord Ltd.
// All rights reserved.
// Use of this source code is governed by a MIT license
// that can be found in the LICENSE file.

package ergonode

import (
	"context"
	"errors"
	"fmt"

	"github.com/halturin/ergonode/etf"
	"github.com/halturin/ergonode/lib"
)

type Process struct {
	local   chan etf.Term
	remote  chan etf.Tuple
	ready   chan bool
	self    etf.Pid
	context context.Context
}

// Behaviour interface contains methods you should implement to make own process behaviour
type ProcessBehaviour interface {
	ProcessLoop(object interface{}, args ...interface{}) // method which implements control flow of process
	Options() (options map[string]interface{})           // method returns process-related options
}

// route incomming message to registered (with sender 'from' value)
func (n *Node) route(from, to etf.Term, message etf.Term) {
	var toPid etf.Pid
	switch tp := to.(type) {
	case etf.Pid:
		toPid = tp
	case etf.Atom:
		toPid, _ = n.registered[tp]
	}
	pcs := n.channels[toPid]
	if from == nil {
		lib.Log("SEND: To: %#v, Message: %#v", to, message)
		pcs.in <- message
	} else {
		lib.Log("REG_SEND: (%#v )From: %#v, To: %#v, Message: %#v", pcs.inFrom, from, to, message)
		pcs.inFrom <- etf.Tuple{from, message}
	}
}

// Send making outgoing message
func (n *Node) Send(from interface{}, to interface{}, message *etf.Term) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.New(fmt.Sprint(r))
		}
	}()

	switch tto := to.(type) {
	case etf.Pid:
		n.sendbyPid(tto, message)
	case etf.Tuple:
		if len(tto) == 2 {
			// causes panic if casting to etf.Atom goes wrong
			if tto[0].(etf.Atom) == tto[1].(etf.Atom) {
				// just stub.
			}
			n.sendbyTuple(from.(etf.Pid), tto, message)
		}
	}

	return nil
}
