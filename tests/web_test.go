package tests

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/ergo-services/ergo"
	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/node"
)

var (
	testWebString = "hello world"
)

type testWebHandler struct {
	gen.WebHandler
}

func (r *testWebHandler) HandleRequest(process *gen.WebHandlerProcess, request gen.WebMessageRequest) gen.WebHandlerStatus {
	request.Response.Write([]byte(testWebString))
	return gen.WebHandlerStatusDone
}

type testWebServer struct {
	gen.Web
}

func (w *testWebServer) InitWeb(process *gen.WebProcess, args ...etf.Term) (gen.WebOptions, error) {
	var options gen.WebOptions

	mux := http.NewServeMux()
	webHandler := process.StartWebHandler(&testWebHandler{}, gen.WebHandlerOptions{})
	mux.Handle("/", webHandler)
	options.Handler = mux

	return options, nil
}

func TestWeb(t *testing.T) {
	fmt.Printf("\n=== Test Web Server\n")
	fmt.Printf("Starting nodes: nodeWeb1@localhost: ")
	node1, err := ergo.StartNode("nodeWeb1@localhost", "cookies", node.Options{})
	defer node1.Stop()
	if err != nil {
		t.Fatal("can't start node", err)
	} else {
		fmt.Println("OK")
	}

	fmt.Printf("...starting process (gen.Web): ")
	_, err = node1.Spawn("web", gen.ProcessOptions{}, &testWebServer{})
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")

	fmt.Printf("...making simple GET request: ")
	res, err := http.Get("http://localhost:8080")
	if err != nil {
		t.Fatal(err)
	}
	out, err := ioutil.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != testWebString {
		t.Fatal("mismatch result")
	}
	fmt.Println("OK")
}
