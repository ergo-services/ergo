package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/halturin/ergo/etf"
	"github.com/halturin/ergo/gen"
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

func (h *Handler) HandleCast(process *gen.ServerProcess, message etf.Term) string {
	fmt.Println("KKKKKKKKKKLL")
	fmt.Println(process.State.(*st).r.URL.Path)
	w := message.(http.ResponseWriter)
	w.Header().Set("Content-Type", "application/json")
	response := response{
		Request: process.State.(*st).r.URL.Path,
		Answer:  "Your request has been handled",
	}

	err := json.NewEncoder(w).Encode(response)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println("KKKKKKKKKK")
	return "stop"
}
