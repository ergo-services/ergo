// Copyright 2012-2013 Metachord Ltd.
// All rights reserved.
// Use of this source code is governed by a MIT license
// that can be found in the LICENSE file.

package ergonode

import (
	"context"

	"github.com/halturin/ergonode/etf"
)

type Process struct {
	local   chan etf.Term
	remote  chan etf.Tuple
	ready   chan bool
	self    etf.Pid
	context context.Context
}

const (
	DefaultProcessMailboxSize = 100
)

// Behaviour interface contains methods you should implement to make own process behaviour
type ProcessBehaviour interface {
	ProcessLoop(process interface{}, args ...interface{}) // method which implements control flow of process
	Options() (options map[string]interface{})            // method returns process-related options
}
