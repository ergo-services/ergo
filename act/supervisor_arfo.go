package act

import (
	"ergo.services/ergo/gen"
)

//
// All|Rest For One implementation
//

func createSupAllRestForOne() supBehavior {
	return &supARFO{
		wait: make(map[gen.PID]bool),
	}
}

type supARFO struct {
	spec []*supChildSpec
	rest bool

	restart      SupervisorRestart
	restarts     []int64
	autoshutdown bool

	mode int // 0 - normal, (1 - starting, 2 - stopping) [re]starting, 3 - shutdown

	keeporder      bool
	shutdownReason error
	restartI       int
	wait           map[gen.PID]bool

	i int
}

func (s *supARFO) init(spec SupervisorSpec) (supAction, error) {
	var action supAction

	if spec.Type == SupervisorTypeRestForOne {
		s.rest = true
	}

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
	s.keeporder = spec.Restart.KeepOrder

	return action, nil
}

func (s *supARFO) childAddSpec(spec SupervisorChildSpec) (supAction, error) {
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

func (s *supARFO) childSpec(name gen.Atom) (supAction, error) {
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

func (s *supARFO) childStarted(cs supChildSpec, pid gen.PID) supAction {
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

func (s *supARFO) childTerminated(name gen.Atom, pid gen.PID, reason error) supAction {
	var action supAction
	var spec *supChildSpec
	var empty gen.PID

	delete(s.wait, pid)

	if s.mode == 3 { // shutdown
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
	// in case we should terminate all children keep the running pids in a map (awaiting termination)
	wait := make(map[gen.PID]bool)
	specI := 0
	for i, cs := range s.spec {
		if cs.Name == name || cs.pid == pid {
			cs.pid = empty
			found = true
			spec = cs
			specI = i
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
		if len(runningChildren) == 0 {
			// no child processes running. just terminate supervisor
			action.reason = reason
			action.do = supActionTerminate
			return action
		}

		// start supervisor termination
		action.terminate = runningChildren
		action.do = supActionTerminateChildren
		action.reason = reason
		s.wait = wait
		s.mode = 3
		s.shutdownReason = reason
		return action
	}

	if s.mode == 2 { // stopping (restarting)

		if s.keeporder == false {
			if len(s.wait) > 0 {
				// return action with empty list. just wait for the child processes
				// to be terminated
				action.do = supActionTerminateChildren
				return action
			}

		} else {
			if len(s.wait) > 0 {
				// must be 0
				panic(gen.ErrInternal)
			}

			if specI < s.restartI {
				// terminated child is not among we are waiting for termination.
				// update the position
				s.restartI = specI
			}

			terminate := s.childrenForTermination()
			if len(terminate) > 0 {
				action.do = supActionTerminateChildren
				action.reason = reason
				action.terminate = terminate
				return action
			}
		}

		s.mode = 1 // starting (restarting)
		action.do = supActionStartChild
		action.spec = s.childForStart()
		s.restartI = 0
		return action
	}

	if spec.disabled {
		// auto shutdown is enabled
		if len(runningChildren) == 0 && s.autoshutdown {
			// there is no more running child processes. start supervisor termination
			action.reason = reason
			action.do = supActionTerminate
			return action
		}
		// do nothing
		return action
	}

	// activate restart strategy
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
			s.mode = 3 // shutdown
			s.shutdownReason = reason
			return action
		}

		// auto shutdown is enabled
		if len(runningChildren) == 0 && s.autoshutdown {
			// there is no more running child processes. start supervisor termination
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
				s.mode = 3 // shutdown
				s.shutdownReason = reason
				return action
			}

			// auto shutdown is enabled
			if len(runningChildren) == 0 && s.autoshutdown {
				// there is no more running child processes. shutting down this supervisor
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

	if exceeded {
		// exceeded intensity. start supervisor termination
		action.terminate = runningChildren
		action.do = supActionTerminateChildren
		action.reason = ErrSupervisorRestartsExceeded
		s.wait = wait
		s.mode = 3 // shutdown
		s.shutdownReason = reason
		return action
	}

	//
	// activate restart strategy
	//

	// set the restarting position
	if s.rest {
		s.restartI = specI // restart from the last to the i-th
	}

	terminate := s.childrenForTermination()
	if len(terminate) == 0 {
		// nothing to stop. start children
		action.do = supActionStartChild
		action.spec = s.childForStart()
		s.mode = 1 // starting (restarting)
		return action

	}
	action.do = supActionTerminateChildren
	action.reason = reason
	action.terminate = terminate
	s.mode = 2 // stopping (restarting)
	return action
}

func (s *supARFO) childEnable(name gen.Atom) (supAction, error) {
	var action supAction
	if s.mode != 0 {
		return action, ErrSupervisorStrategyActive
	}

	for _, cs := range s.spec {
		if cs.Name != name {
			continue
		}

		if cs.disabled == false {
			// do nothing. its already enabled
			return action, nil
		}

		// it was disabled. enable it and start child process with this spec

		action.do = supActionStartChild
		action.spec = *cs

		return action, nil
	}

	return action, ErrSupervisorChildUnknown
}

func (s *supARFO) childDisable(name gen.Atom) (supAction, error) {
	var action supAction
	var empty gen.PID

	if s.mode != 0 {
		return action, ErrSupervisorStrategyActive
	}

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
		s.wait[cs.pid] = true
		return action, nil
	}

	return action, ErrSupervisorChildUnknown
}

func (s *supARFO) children() []SupervisorChild {
	var c []supChild

	for _, cs := range s.spec {
		c = append(c, supChild{cs.pid, *cs})
	}
	return sortSupChild(c)
}

func (s *supARFO) childrenForTermination() []gen.PID {
	var terminate []gen.PID
	var empty gen.PID

	for i := range s.spec {
		// in reverse order
		k := len(s.spec) - 1 - i
		if k < s.restartI {
			break
		}
		if s.spec[k].disabled {
			continue
		}
		if s.spec[k].pid == empty {
			continue
		}

		pid := s.spec[k].pid
		s.wait[pid] = true
		terminate = append(terminate, pid)
		if s.keeporder {
			// only the last one
			break
		}
	}
	return terminate
}

func (s *supARFO) childForStart() supChildSpec {
	var empty gen.PID

	// get the first enabled spec
	for _, cs := range s.spec[s.restartI:] {
		if cs.disabled {
			continue
		}

		if cs.pid != empty {
			// shouldn't be running child processes in the range of s.spec[s.restartI:]
			panic(gen.ErrInternal)
		}

		return *cs
	}

	panic(gen.ErrInternal)
}
