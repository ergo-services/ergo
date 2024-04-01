package node

import (
	"sync"

	"ergo.services/ergo/gen"
)

func createTarget() *target {
	return &target{}
}

type consumers struct {
	sync.RWMutex
	list []gen.PID
}

type target struct {
	c sync.Map // consumers
}

// returns true if registered first consumer of the target
func (t *target) registerConsumer(target any, consumer gen.PID) bool {
	pc := &consumers{}
	first := true

	if value, exist := t.c.LoadOrStore(target, pc); exist {
		pc = value.(*consumers)
		first = false
	}

	pc.Lock()
	defer pc.Unlock()
	pc.list = append(pc.list, consumer)

	return first
}

// returns true if unregistered consumer was the last one
func (t *target) unregisterConsumer(target any, consumer gen.PID) bool {
	value, exist := t.c.Load(target)
	if exist == false {
		return false
	}

	pc := value.(*consumers)
	pc.Lock()
	defer pc.Unlock()

	for i, pid := range pc.list {
		if pid != consumer {
			continue
		}
		pc.list[0] = pc.list[i]
		pc.list = pc.list[1:]
		break
	}
	return false
}

func (t *target) unregister(target any) []gen.PID {
	var list []gen.PID

	value, exist := t.c.LoadAndDelete(target)
	if exist == false {
		return list
	}
	pc := value.(*consumers)
	return pc.list
}

func (t *target) targetsNodeDown(node gen.Atom) []any {
	var targets []any
	t.c.Range(func(k, v any) bool {
		switch tt := k.(type) {
		case gen.PID:
			if tt.Node == node {
				targets = append(targets, tt)
				return true
			}
		case gen.ProcessID:
			if tt.Node == node {
				targets = append(targets, tt)
				return true
			}
		case gen.Alias:
			if tt.Node == node {
				targets = append(targets, k)
				return true
			}
		case gen.Event:
			if tt.Node == node {
				targets = append(targets, k)
				return true
			}
		case gen.Atom:
			if tt == node {
				targets = append(targets, k)
				return true
			}
		}
		// remove remote consumers (belonging to the node that went down)
		pc := v.(*consumers)
		pc.Lock()
		list := []gen.PID{}
		for _, pid := range pc.list {
			if pid.Node == node {
				continue
			}
			list = append(list, pid)
		}
		pc.list = list
		pc.Unlock()

		return true
	})
	return targets
}

func (t *target) consumers(target any) []gen.PID {
	var list []gen.PID

	value, exist := t.c.Load(target)
	if exist == false {
		return list
	}
	pc := value.(*consumers)

	pc.RLock()
	defer pc.RUnlock()

	return pc.list
}
