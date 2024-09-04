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
		spec: make(map[gen.Atom]*supChildSpec),
		pids: make(map[gen.PID]*supChildSpec),
	}
}

type supSOFO struct {
	spec map[gen.Atom]*supChildSpec
	pids map[gen.PID]*supChildSpec

	restart  SupervisorRestart
	restarts []int64

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
		s.i++
		s.spec[cs.Name] = &cs
	}
	s.wait = make(map[gen.PID]bool)
	return action, nil
}

func (s *supSOFO) childAddSpec(spec SupervisorChildSpec) (supAction, error) {
	var action supAction

	if s.shutdown {
		return action, fmt.Errorf("shutting down")
	}

	if err := validateChildSpec(spec); err != nil {
		return action, err
	}
	if _, duplicate := s.spec[spec.Name]; duplicate {
		return action, ErrSupervisorChildDuplicate
	}

	cs := supChildSpec{
		SupervisorChildSpec: spec,
	}
	cs.i = s.i
	s.i++
	s.spec[cs.Name] = &cs

	// SOFO doesn't start it on adding, so do nothing
	return action, nil
}

func (s *supSOFO) childSpec(name gen.Atom) (supAction, error) {
	var action supAction

	if s.shutdown {
		return action, nil
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

	sc, found := s.spec[spec.Name]
	if found == false {
		// do nothing
		return action
	}

	// do not overwrite args since it is a dynamic child
	// sc.Args = spec.Args

	// keep it and do nothing
	s.pids[pid] = sc
	return action
}

func (s *supSOFO) childTerminated(name gen.Atom, pid gen.PID, reason error) supAction {
	var action supAction

	delete(s.pids, pid)

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
		switch s.restart.Strategy {
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

		// check for restart intensity
		restarts, exceeded := supCheckRestartIntensity(s.restarts,
			int(s.restart.Period),
			int(s.restart.Intensity))
		s.restarts = restarts

		if exceeded == false {
			// do restart
			action.do = supActionStartChild
			action.spec = *spec

			return action
		}

		// exceeded intensity. start termination
		action.do = supActionTerminateChildren
		action.reason = ErrSupervisorRestartsExceeded
	} else {
		action.do = supActionTerminateChildren
		action.reason = reason
	}

	for pid := range s.pids {
		action.terminate = append(action.terminate, pid)
		s.wait[pid] = true
	}
	s.shutdown = true
	s.shutdownReason = action.reason
	return action
}

func (s *supSOFO) childEnable(name gen.Atom) (supAction, error) {
	var action supAction

	if s.shutdown {
		return action, fmt.Errorf("shutting down")
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
		return action, fmt.Errorf("shutting down")
	}

	spec, found := s.spec[name]
	if found == false {
		return action, ErrSupervisorChildUnknown
	}
	spec.disabled = true

	terminate := []gen.PID{}
	for pid, spec := range s.pids {
		if spec.Name != name {
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
	for pid, spec := range s.pids {
		c = append(c, supChild{pid, *spec})
	}
	return sortSupChild(c)
}
