package act

import (
	"fmt"

	"ergo.services/ergo/gen"
)

//
// Simple One For One implementation
//

func createSupSimpleOneForOne() supBehavior {
	return &supSOFO{
		spec:      make(map[gen.Atom]*supChildSpec),
		instances: make(map[gen.PID]*sofoInstance),
	}
}

// sofoInstance carries the state of a logical SOFO instance across
// restarts: identifying spec, instance args and the per-instance
// restart history (only used when the spec opts in via Restart.Intensity).
type sofoInstance struct {
	spec     gen.Atom
	args     []any
	restarts []int64
}

type supSOFO struct {
	spec      map[gen.Atom]*supChildSpec
	instances map[gen.PID]*sofoInstance

	restart  SupervisorRestart
	restarts []int64

	// pendingInstance carries the sofoInstance over a restart spawn so its
	// per-instance counter and args survive the new pid.
	pendingInstance *sofoInstance

	history []supRestartEvent

	i              int
	shutdown       bool
	shutdownReason error
	wait           map[gen.PID]bool
}

func (s *supSOFO) init(spec SupervisorSpec) (supAction, error) {
	var action supAction

	s.restart = spec.Restart
	for _, c := range spec.Children {
		cs := supChildSpec{
			SupervisorChildSpec: c,
		}
		cs.i = s.i
		cs.effStrategy = resolveStrategy(s.restart.Strategy, c.Restart.Strategy)
		s.i++
		s.spec[cs.Name] = &cs
	}
	s.wait = make(map[gen.PID]bool)
	return action, nil
}

func (s *supSOFO) childAddSpec(spec SupervisorChildSpec) (supAction, error) {
	var action supAction

	if s.shutdown {
		return action, ErrSupervisorStrategyActive
	}

	if err := validateChildSpec(spec); err != nil {
		return action, err
	}
	if err := validateChildRestart(spec.Restart, SupervisorTypeSimpleOneForOne); err != nil {
		return action, fmt.Errorf("%w: %s", ErrSupervisorInvalidSpec, err)
	}
	if _, duplicate := s.spec[spec.Name]; duplicate {
		return action, ErrSupervisorChildDuplicate
	}

	cs := supChildSpec{
		SupervisorChildSpec: spec,
	}
	cs.i = s.i
	cs.effStrategy = resolveStrategy(s.restart.Strategy, spec.Restart.Strategy)
	s.i++
	s.spec[cs.Name] = &cs

	// SOFO doesn't start it on adding, so do nothing
	return action, nil
}

func (s *supSOFO) childSpec(name gen.Atom) (supAction, error) {
	var action supAction

	if s.shutdown {
		return action, ErrSupervisorStrategyActive
	}

	spec, found := s.spec[name]
	if found == false {
		return action, ErrSupervisorChildUnknown
	}

	if spec.disabled {
		return action, ErrSupervisorChildDisabled
	}

	action.do = supActionStartChild
	action.spec = *spec
	return action, nil
}

func (s *supSOFO) childStarted(spec supChildSpec, pid gen.PID) supAction {
	var action supAction

	if s.shutdown {
		return action
	}

	if _, found := s.spec[spec.Name]; found == false {
		// do nothing
		return action
	}

	if s.pendingInstance != nil {
		// restart path: carry over the sofoInstance from the previous pid
		s.instances[pid] = s.pendingInstance
		s.pendingInstance = nil
		return action
	}

	// brand-new instance via StartChild
	s.instances[pid] = &sofoInstance{
		spec: spec.Name,
		args: spec.Args,
	}
	return action
}

func (s *supSOFO) childTerminated(name gen.Atom, pid gen.PID, reason error) supAction {
	var action supAction

	inst, hasInst := s.instances[pid]
	delete(s.instances, pid)

	if s.shutdown {
		delete(s.wait, pid)
		if len(s.wait) > 0 {
			// return action with empty process list for termination
			action.do = supActionTerminateChildren
			return action
		}

		// children terminated. shutdown the supervisor
		action.do = supActionTerminate
		action.reason = s.shutdownReason
		return action
	}

	spec, found := s.spec[name]
	if found {

		// check strategy
		switch spec.effStrategy {
		case SupervisorStrategyTemporary:
			// do nothing
			return action
		case SupervisorStrategyTransient:
			if reason == gen.TerminateReasonNormal || reason == gen.TerminateReasonShutdown {
				// do nothing
				return action
			}
		}

		if spec.disabled {
			// do nothing
			return action
		}

		// pick the counter: per-instance if opted in, otherwise the global one
		var (
			restarts []int64
			exceeded bool
			period   = int(s.restart.Period)
			intens   = int(s.restart.Intensity)
			useLocal = spec.Restart.Intensity > 0 && hasInst
		)
		if useLocal {
			period = int(spec.Restart.Period)
			if period == 0 {
				period = int(defaultRestartPeriod)
			}
			intens = int(spec.Restart.Intensity)
			restarts, exceeded = supCheckRestartIntensity(inst.restarts, period, intens)
			inst.restarts = restarts
		} else {
			restarts, exceeded = supCheckRestartIntensity(s.restarts, period, intens)
			s.restarts = restarts
		}

		if exceeded == false {
			s.history = supAppendHistory(s.history, spec.Name, reason)
			// do restart and carry the instance state to the new pid
			action.do = supActionStartChild
			action.spec = *spec
			if hasInst && len(inst.args) > 0 {
				action.spec.Args = inst.args
			}
			if hasInst {
				s.pendingInstance = inst
			}
			action.adoptMailbox = extractMailbox(reason)
			return action
		}

		// exceeded. only per-instance counter can drop just this instance
		if useLocal && spec.Restart.OnExceed == OnExceedDisable {
			// instance already removed at the top; supervisor stays alive
			return action
		}

		action.do = supActionTerminateChildren
		action.reason = gen.ErrExceeded
		s.shutdownReason = gen.Errorf("supervisor restart intensity exceeded (max %d in %ds): %w: %w",
			intens, period, gen.ErrExceeded, reason)
	} else {
		action.do = supActionTerminateChildren
		action.reason = reason
		s.shutdownReason = reason
	}

	for pid := range s.instances {
		action.terminate = append(action.terminate, pid)
		s.wait[pid] = true
	}
	s.shutdown = true

	if len(action.terminate) == 0 {
		action.do = supActionTerminate
		action.reason = s.shutdownReason
	}
	return action
}

func (s *supSOFO) childEnable(name gen.Atom) (supAction, error) {
	var action supAction

	if s.shutdown {
		return action, ErrSupervisorStrategyActive
	}

	spec, found := s.spec[name]
	if found == false {
		return action, ErrSupervisorChildUnknown
	}
	spec.disabled = false
	return action, nil
}

func (s *supSOFO) childDisable(name gen.Atom) (supAction, error) {
	var action supAction

	if s.shutdown {
		return action, ErrSupervisorStrategyActive
	}

	spec, found := s.spec[name]
	if found == false {
		return action, ErrSupervisorChildUnknown
	}
	spec.disabled = true

	terminate := []gen.PID{}
	for pid, inst := range s.instances {
		if inst.spec != name {
			continue
		}
		terminate = append(terminate, pid)
		s.wait[pid] = true
	}

	if len(terminate) > 0 {
		action.do = supActionTerminateChildren
		action.reason = gen.TerminateReasonShutdown
		action.terminate = terminate
	}
	return action, nil
}

func (s *supSOFO) children() []SupervisorChild {
	var c []supChild
	for pid, inst := range s.instances {
		spec, ok := s.spec[inst.spec]
		if ok == false {
			continue
		}
		c = append(c, supChild{pid, *spec})
	}
	return sortSupChild(c)
}

func (s *supSOFO) inspect(items ...string) map[string]string {
	result := make(map[string]string)

	result["type"] = "Simple One For One"
	result["strategy"] = s.restart.Strategy.String()
	result["intensity"] = fmt.Sprintf("%d", s.restart.Intensity)
	result["period"] = fmt.Sprintf("%d", s.restart.Period)
	result["restarts_count"] = fmt.Sprintf("%d", len(s.restarts))

	specsTotal := len(s.spec)
	specsDisabled := 0

	instancesPerSpec := make(map[gen.Atom]int)
	instancesWithArgsPerSpec := make(map[gen.Atom]int)
	localRestartsPerSpec := make(map[gen.Atom]int)

	for _, inst := range s.instances {
		instancesPerSpec[inst.spec]++
		if len(inst.args) > 0 {
			instancesWithArgsPerSpec[inst.spec]++
		}
		localRestartsPerSpec[inst.spec] += len(inst.restarts)
	}

	for name, cs := range s.spec {
		if cs.disabled {
			specsDisabled++
		}
		count := instancesPerSpec[name]
		countWithArgs := instancesWithArgsPerSpec[name]
		result[fmt.Sprintf("child:%s", name)] = fmt.Sprintf("%d", count)
		result[fmt.Sprintf("child:%s:args", name)] = fmt.Sprintf("%d", countWithArgs)
		if cs.Restart.Intensity > 0 {
			result[fmt.Sprintf("child:%s:restarts", name)] = fmt.Sprintf("%d", localRestartsPerSpec[name])
		}
	}

	result["specs_total"] = fmt.Sprintf("%d", specsTotal)
	result["specs_disabled"] = fmt.Sprintf("%d", specsDisabled)
	result["instances_total"] = fmt.Sprintf("%d", len(s.instances))

	supHistoryToInspect(s.history, result)

	return result
}
