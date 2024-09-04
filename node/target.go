package node

import (
	"sync"

	"ergo.services/ergo/gen"
)

func createTarget() *target {
	return &target{
		c: make(map[any][]gen.PID),
	}
}

type target struct {
	c map[any][]gen.PID // consumers
	sync.RWMutex
}

// returns true if registered first consumer of the target
func (t *target) registerConsumer(target any, consumer gen.PID) bool {
	t.Lock()
	defer t.Unlock()

	list := t.c[target]
	list = append(list, consumer)
	t.c[target] = list

	first := len(list) == 1
	return first
}

// returns true if unregistered consumer was the last one
func (t *target) unregisterConsumer(target any, consumer gen.PID) bool {
	t.Lock()
	defer t.Unlock()

	list, exist := t.c[target]
	if exist == false {
		return false
	}

	for i, pid := range list {
		if pid != consumer {
			continue
		}
		list[0] = list[i]
		list = list[1:]
		if len(list) == 0 {
			delete(t.c, target)
			return true
		}
		t.c[target] = list
		break
	}

	return false
}

func (t *target) unregister(target any) []gen.PID {
	t.Lock()
	defer t.Unlock()

	list, exist := t.c[target]
	if exist == false {
		return list
	}
	delete(t.c, target)
	return list
}

func (t *target) targetsNodeDown(node gen.Atom) []any {
	var targets []any

	t.Lock()
	defer t.Unlock()

	for k, list := range t.c {
		switch tt := k.(type) {
		case gen.PID:
			if tt.Node == node {
				targets = append(targets, tt)
			}
		case gen.ProcessID:
			if tt.Node == node {
				targets = append(targets, tt)
			}
		case gen.Alias:
			if tt.Node == node {
				targets = append(targets, k)
			}
		case gen.Event:
			if tt.Node == node {
				targets = append(targets, k)
			}
		case gen.Atom:
			if tt == node {
				targets = append(targets, k)
			}
		}

		// remove remote consumers (belonging to the node that went down)
		newlist := []gen.PID{}
		for _, pid := range list {
			if pid.Node == node {
				// skip it
				continue
			}
			newlist = append(newlist, pid)
		}

		t.c[k] = newlist
	}

	return targets
}

func (t *target) consumers(target any) []gen.PID {
	t.RLock()
	defer t.RUnlock()
	return t.c[target]
}
