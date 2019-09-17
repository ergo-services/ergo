package ergonode

// start node1
// start n1gs1

// check node1.registrar for pid and 'n1gs1' name
// start node2
// monitor n1gs1 -> node2
// check node1.registrar for peer node2
// check node2.registrar for peer node1

import (
	"testing"
)

func TestCreateRegistrar(t *testing.T) {
	// node := CreateNode("node@localhost","cookies", NodeOptions{})
}
