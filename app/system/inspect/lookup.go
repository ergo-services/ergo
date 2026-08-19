package inspect

import "ergo.services/ergo/gen"

// responseProcessLookup resolves a name to a PID or a PID to a name, and reports
// the state either way. A remote caller sees names in logs and listings, while
// every action takes a PID, so without this it would have to scan a process list.
func (i *inspect) responseProcessLookup(request RequestGetProcessLookup) ResponseGetProcessLookup {
	response := ResponseGetProcessLookup{
		PID:  request.PID,
		Name: request.Name,
	}

	switch {
	case request.Name != "":
		pid, err := i.Node().ProcessPID(request.Name)
		if err != nil {
			response.Error = err
			return response
		}
		response.PID = pid

	case request.PID != (gen.PID{}):
		// the name is looked up below; an unregistered process is not an error
		break

	default:
		response.Error = gen.ErrIncorrect
		return response
	}

	state, err := i.Node().ProcessState(response.PID)
	if err != nil {
		response.Error = err
		return response
	}
	response.State = state

	if response.Name == "" {
		if name, err := i.Node().ProcessName(response.PID); err == nil {
			response.Name = name
		}
	}

	return response
}
