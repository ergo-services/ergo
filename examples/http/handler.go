package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
)

// GenServer implementation structure
type Handler struct {
	gen.Server
}

type st struct {
	r *http.Request
}

type response struct {
	Request string
	Answer  string
}

// Init initializes process state using arbitrary arguments
// Init(...) -> state
func (h *Handler) Init(process *gen.ServerProcess, args ...etf.Term) error {
	fmt.Println("Start handling http request")
	process.State = &st{
		r: args[0].(*http.Request),
	}
	return nil
}

func (h *Handler) HandleInfo(process *gen.ServerProcess, message etf.Term) gen.ServerStatus {
	fmt.Println(process.State.(*st).r.URL.Path)
	w := message.(http.ResponseWriter)
	w.Header().Set("Content-Type", "application/json")
	response := response{
		Request: process.State.(*st).r.URL.Path,
		Answer:  "Your request has been handled by " + process.Self().String(),
	}

	err := json.NewEncoder(w).Encode(response)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println("Finish handling http request and stop the server", process.Self())
	return gen.ServerStatusStop
}
