//+build node

package ergonode

import (
	"testing"
	"time"
)


func TestNode(t *testing.T) {

	node := CreateNode("node@localhost","cookies", NodeOptions{})
	node.Stop()
}
