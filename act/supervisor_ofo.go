package act

import (
	"ergo.services/ergo/gen"
)

//
// One For One implementation
//

func createSupOneForOne() supBehavior {
	return &supOFO{}
}

type supOFO struct {
	spec []*supChildSpec

	restart      SupervisorRestart
	restarts     []int64
	autoshutdown bool

	mode int // 0 - normal, (1 - starting, 2 - stopping) [re]starting

	shutdown       bool
	shutdownReason error
	wait           map[gen.PID]bool

	i int
}

func (s *supOFO) init(spec SupervisorSpec) (supAction, error) {
	var action supAction

	s.restart = spec.Restart
	for _, c := range spec.Children {
		cs := supChildSpec{
			SupervisorChildSpec: c,
		}
		cs.register = true
		cs.i = s.i
		s.i++
		s.spec = append(s.spec, &cs)
	}

	action.do = supActionStartChild
	action.spec = *s.spec[0]

	s.mode = 1 // starting
	s.autoshutdown = spec.DisableAutoShutdown == false

	return action, nil
}

func (s *supOFO) childAddSpec(spec SupervisorChildSpec) (supAction, error) {
	var action supAction

	if s.mode != 0 {
		return action, ErrSupervisorStrategyActive
	}

	if err := validateChildSpec(spec); err != nil {
		return action, err
	}

	for _, cs := range s.spec {
		if cs.Name == spec.Name {
			return action, ErrSupervisorChildDuplicate
		}
	}

	cs := supChildSpec{
		SupervisorChildSpec: spec,
	}
	cs.register = true
	cs.i = s.i
	s.i++
	s.spec = append(s.spec, &cs)

	// start this child
	action.do = supActionStartChild
	action.spec = cs

	return action, nil
}

func (s *supOFO) childSpec(name gen.Atom) (supAction, error) {
	var action supAction
	var empty gen.PID

	// single start (if it was terminated normally before)

	if s.mode != 0 {
		return action, ErrSupervisorStrategyActive
	}

	for _, spec := range s.spec {
		if spec.Name != name {
			continue
		}

		if spec.disabled {
			return action, ErrSupervisorChildDisabled
		}

		if spec.pid == empty {
			action.do = supActionStartChild
			action.spec = *spec
			return action, nil
		}

		// already running
		return action, ErrSupervisorChildRunning
	}

	return action, ErrSupervisorChildUnknown
}

func (s *supOFO) childStarted(cs supChildSpec, pid gen.PID) supAction {
	var action supAction
	var empty gen.PID

	// let panic if got unknown child
	spec := s.spec[cs.i]
	if cs.Name != spec.Name {
		panic(gen.ErrInternal)
	}

	// update args, keep the pid and do nothing
	spec.Args = cs.Args
	spec.pid = pid

	if s.mode != 1 { // is not in starting mode?
		// do nothing
		return action
	}

	// starting mode. start the rest

	if cs.i == len(s.spec)-1 {
		// it was the last spec. do nothing more
		s.mode = 0 // normal
		return action
	}

	// check the rest children if they are running
	for i := cs.i + 1; i < len(s.spec); i++ {
		if s.spec[i].pid != empty {
			continue
		}

		if s.spec[i].disabled {
			continue
		}

		action.do = supActionStartChild
		action.spec = *s.spec[i]
		action.spec.i = i
		return action
	}

	return action
}

func (s *supOFO) childTerminated(name gen.Atom, pid gen.PID, reason error) supAction {
	var action supAction
	var spec *supChildSpec
	var empty gen.PID

	delete(s.wait, pid)

	if s.shutdown {
		if len(s.wait) > 0 {
			// return action with empty list
			action.do = supActionTerminateChildren
			return action
		}

		action.do = supActionTerminate
		action.reason = s.shutdownReason
		return action
	}

	found := false
	runningChildren := []gen.PID{}
	wait := make(map[gen.PID]bool)
	for _, cs := range s.spec {
		if cs.Name == name || cs.pid == pid {
			cs.pid = empty
			found = true
			spec = cs
			continue
		}

		if cs.pid == empty {
			continue
		}

		runningChildren = append(runningChildren, cs.pid)
		wait[cs.pid] = true
	}

	if found == false {
		// seems supervisor got exit-signal from a non-child process.
		// start supervisor termination
		if len(runningChildren) == 0 {
			action.reason = reason
			action.do = supActionTerminate
			return action
		}
		action.terminate = runningChildren
		action.do = supActionTerminateChildren
		action.reason = reason
		s.wait = wait
		s.shutdown = true
		s.shutdownReason = reason
		return action
	}

	if spec.disabled {
		// auto shutdown is enabled
		if len(runningChildren) == 0 && s.autoshutdown {
			// there is no more running child processes. terminate supervisor
			action.reason = reason
			action.do = supActionTerminate
			return action
		}
		// do nothing
		return action
	}

	// check strategy
	switch s.restart.Strategy {
	case SupervisorStrategyTemporary:
		if spec.Significant {
			// significant child has terminated.
			if len(runningChildren) == 0 {
				action.reason = reason
				action.do = supActionTerminate
				return action
			}

			action.terminate = runningChildren
			action.do = supActionTerminateChildren
			action.reason = reason
			s.wait = wait
			s.shutdown = true
			s.shutdownReason = reason
			return action
		}

		// auto shutdown is enabled
		if len(runningChildren) == 0 && s.autoshutdown {
			// there is no more running child processes. terminate supervisor
			action.reason = reason
			action.do = supActionTerminate
			return action
		}

		// do nothing
		return action

	case SupervisorStrategyTransient:
		if reason == gen.TerminateReasonNormal || reason == gen.TerminateReasonShutdown {
			if spec.Significant {
				// significant child has terminated
				if len(runningChildren) == 0 {
					action.reason = reason
					action.do = supActionTerminate
					return action
				}

				action.terminate = runningChildren
				action.do = supActionTerminateChildren
				action.reason = reason

				s.wait = wait
				s.shutdown = true
				s.shutdownReason = reason
				return action
			}

			// auto shutdown is enabled
			if len(runningChildren) == 0 && s.autoshutdown {
				// there is no more running child processes. terminate supervisor
				action.reason = reason
				action.do = supActionTerminate
				return action
			}

			// do nothing
			return action
		}
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
	for _, cs := range s.spec {
		if cs.pid == empty {
			continue
		}
		action.terminate = append(action.terminate, cs.pid)
	}
	action.do = supActionTerminateChildren
	action.reason = ErrSupervisorRestartsExceeded
	s.wait = wait
	s.shutdown = true
	s.shutdownReason = reason

	return action
}

func (s *supOFO) childEnable(name gen.Atom) (supAction, error) {
	var action supAction
	for _, cs := range s.spec {
		if cs.Name != name {
			continue
		}

		if cs.disabled == false {
			// do nothing. its already enabled
			return action, nil
		}

		// it was disabled. enable it and start child process with this spec
		cs.disabled = false

		action.do = supActionStartChild
		action.spec = *cs

		return action, nil
	}

	return action, ErrSupervisorChildUnknown
}

func (s *supOFO) childDisable(name gen.Atom) (supAction, error) {
	var action supAction
	var empty gen.PID

	for _, cs := range s.spec {
		if cs.Name != name {
			continue
		}

		if cs.disabled {
			// do nothing. its already disabled
			return action, nil
		}

		if cs.pid == empty {
			return action, nil
		}

		cs.disabled = true
		action.do = supActionTerminateChildren
		action.terminate = []gen.PID{cs.pid}
		action.reason = gen.TerminateReasonShutdown
		return action, nil
	}

	return action, ErrSupervisorChildUnknown
}

func (s *supOFO) children() []SupervisorChild {
	var c []supChild

	for _, cs := range s.spec {
		c = append(c, supChild{cs.pid, *cs})
	}
	return sortSupChild(c)
}
