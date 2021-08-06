package main

import (
	"flag"
	"fmt"
	"net/http"

	"github.com/halturin/ergo"
)

var (
	NodeName         string
	Cookie           string
	ListenRangeBegin int
	ListenRangeEnd   int = 35000
	Listen           string
	ListenEPMD       int
)

func init() {
	flag.IntVar(&ListenRangeBegin, "listen_begin", 15151, "listen port range")
	flag.IntVar(&ListenRangeEnd, "listen_end", 25151, "listen port range")
	flag.StringVar(&NodeName, "name", "web@127.0.0.1", "node name")
	flag.IntVar(&ListenEPMD, "epmd", 4369, "EPMD port")
	flag.StringVar(&Cookie, "cookie", "123", "cookie for interaction with erlang cluster")
}

func main() {
	flag.Parse()

	opts := ergo.NodeOptions{
		ListenRangeBegin: uint16(ListenRangeBegin),
		ListenRangeEnd:   uint16(ListenRangeEnd),
		EPMDPort:         uint16(ListenEPMD),
	}

	// Initialize new node with given name, cookie, listening port range and epmd port
	node, _ := ergo.CreateNode(NodeName, Cookie, opts)

	// start application
	if err := node.ApplicationLoad(&App{}); err != nil {
		panic(err)
	}

	process, _ := node.ApplicationStart("WebApp")

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		p := process.Node.GetProcessByName("handler_sup")

		if pid, err := handler_sup.StartChild(p, "handler", r); err == nil {
			process.Cast(pid, w)
			handler := process.Node.GetProcessByPid(pid)
			handler.Wait()
			return
		}

		w.WriteHeader(http.StatusInternalServerError)
	})

	go http.ListenAndServe(":8080", nil)
	fmt.Println("HTTP is listening on http://127.0.0.1:8080")

	process.Wait()
	node.Stop()
}
