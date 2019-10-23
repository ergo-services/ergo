package ergonode

import (
	"fmt"
	"testing"
)

func TestApplication(t *testing.T) {

	node := CreateNode("nodeApplication@localhost", "cookies", NodeOptions{})
	if node == nil {
		t.Fatal("can't start node")
	} else {
		fmt.Println("OK")
	}

	node.Stop()
}
