package tm

import (
	"sync"
	"sync/atomic"

	"ergo.services/ergo/gen"
)

type Relation struct {
	Consumer gen.PID
	Kind     Kind
}

type indexKey struct {
	consumer gen.PID
	kind     Kind
}

type targetEntry struct {
	relations *TargetRelations
	index     sync.Map // indexKey -> *relationItem
}

type consumerEntry struct {
	links    sync.Map // target any -> struct{}
	monitors sync.Map // target any -> struct{}
}

type Storage struct {
	targets sync.Map // any -> *targetEntry
	reverse sync.Map // gen.PID -> *consumerEntry

	linkCount    atomic.Int64
	monitorCount atomic.Int64
}

func NewStorage() *Storage {
	return &Storage{}
}

func (s *Storage) Counts() (links int64, monitors int64) {
	return s.linkCount.Load(), s.monitorCount.Load()
}

// Returns false if (consumer, kind) was already registered.
func (s *Storage) Register(target any, consumer gen.PID, kind Kind) bool {
	entry := s.getOrCreateEntry(target)
	item := entry.relations.Push(consumer, kind)
	_, loaded := entry.index.LoadOrStore(indexKey{consumer: consumer, kind: kind}, item)
	if loaded {
		entry.relations.MarkDead(item)
		return false
	}
	s.reverseAdd(consumer, target, kind)
	s.bumpCount(kind, +1)
	return true
}

// Decrements counters only when this caller wins the MarkDead CAS;
// a concurrent RemoveTarget walk may have already claimed the item.
func (s *Storage) Unregister(target any, consumer gen.PID, kind Kind) bool {
	v, ok := s.targets.Load(target)
	if ok == false {
		return false
	}
	entry := v.(*targetEntry)
	iv, ok := entry.index.LoadAndDelete(indexKey{consumer: consumer, kind: kind})
	if ok == false {
		return false
	}
	if entry.relations.MarkDead(iv.(*relationItem)) == false {
		s.reverseDelete(consumer, target, kind)
		return true
	}
	s.reverseDelete(consumer, target, kind)
	s.bumpCount(kind, -1)
	entry.relations.compact()
	return true
}

func (s *Storage) Has(target any, consumer gen.PID, kind Kind) bool {
	v, ok := s.targets.Load(target)
	if ok == false {
		return false
	}
	_, ok = v.(*targetEntry).index.Load(indexKey{consumer: consumer, kind: kind})
	return ok
}

func (s *Storage) Walk(target any, fn func(consumer gen.PID, kind Kind)) {
	v, ok := s.targets.Load(target)
	if ok == false {
		return
	}
	v.(*targetEntry).relations.Walk(fn)
}

func (s *Storage) LinksFor(consumer gen.PID) []any {
	v, ok := s.reverse.Load(consumer)
	if ok == false {
		return nil
	}
	var out []any
	v.(*consumerEntry).links.Range(func(k, _ any) bool {
		out = append(out, k)
		return true
	})
	return out
}

func (s *Storage) MonitorsFor(consumer gen.PID) []any {
	v, ok := s.reverse.Load(consumer)
	if ok == false {
		return nil
	}
	var out []any
	v.(*consumerEntry).monitors.Range(func(k, _ any) bool {
		out = append(out, k)
		return true
	})
	return out
}

func (s *Storage) ClearConsumer(consumer gen.PID) {
	s.reverse.Delete(consumer)
}

// Death fan-out goes through the relations list (Drain), claiming each live
// relation via CAS; a concurrent Unregister that wins the CAS handles its own
// decrement. Entry is detached first, so a later Unregister is a no-op.
func (s *Storage) RemoveTarget(target any) []Relation {
	v, ok := s.targets.LoadAndDelete(target)
	if ok == false {
		return nil
	}
	entry := v.(*targetEntry)
	var out []Relation
	var links, monitors int64
	entry.relations.Drain(func(pid gen.PID, kind Kind) {
		out = append(out, Relation{Consumer: pid, Kind: kind})
		s.reverseDelete(pid, target, kind)
		if kind == KindLink {
			links++
			return
		}
		monitors++
	})
	s.linkCount.Add(-links)
	s.monitorCount.Add(-monitors)
	return out
}

func (s *Storage) RangeTargets(fn func(target any) bool) {
	s.targets.Range(func(k, _ any) bool {
		return fn(k)
	})
}

func (s *Storage) RemoveConsumersOnNode(target any, node gen.Atom) int {
	v, ok := s.targets.Load(target)
	if ok == false {
		return 0
	}
	entry := v.(*targetEntry)
	var links, monitors int
	entry.index.Range(func(k, val any) bool {
		key := k.(indexKey)
		if key.consumer.Node != node {
			return true
		}
		if entry.index.CompareAndDelete(k, val) == false {
			return true
		}
		if entry.relations.MarkDead(val.(*relationItem)) == false {
			s.reverseDelete(key.consumer, target, key.kind)
			return true
		}
		s.reverseDelete(key.consumer, target, key.kind)
		if key.kind == KindLink {
			links++
			return true
		}
		monitors++
		return true
	})
	s.linkCount.Add(-int64(links))
	s.monitorCount.Add(-int64(monitors))
	entry.relations.compact()
	return links + monitors
}

func (s *Storage) bumpCount(kind Kind, delta int64) {
	if kind == KindLink {
		s.linkCount.Add(delta)
		return
	}
	s.monitorCount.Add(delta)
}

func (s *Storage) reverseAdd(consumer gen.PID, target any, kind Kind) {
	v, ok := s.reverse.Load(consumer)
	if ok == false {
		ce := &consumerEntry{}
		actual, _ := s.reverse.LoadOrStore(consumer, ce)
		v = actual
	}
	ce := v.(*consumerEntry)
	if kind == KindLink {
		ce.links.Store(target, struct{}{})
		return
	}
	ce.monitors.Store(target, struct{}{})
}

func (s *Storage) reverseDelete(consumer gen.PID, target any, kind Kind) {
	v, ok := s.reverse.Load(consumer)
	if ok == false {
		return
	}
	ce := v.(*consumerEntry)
	if kind == KindLink {
		ce.links.Delete(target)
		return
	}
	ce.monitors.Delete(target)
}

func (s *Storage) getOrCreateEntry(target any) *targetEntry {
	if v, ok := s.targets.Load(target); ok {
		return v.(*targetEntry)
	}
	entry := &targetEntry{relations: NewTargetRelations()}
	actual, _ := s.targets.LoadOrStore(target, entry)
	return actual.(*targetEntry)
}
