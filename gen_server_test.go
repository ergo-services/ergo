//+build genserver

package ergonode

import (
	"testing"
	"time"
)


func TestGenServer(t *testing.T) {

	node := CreateNode("node@localhost","cookies", NodeOptions{})
	g:=&observer{}
	process_opts := map[string]interface{}{
		"mailbox-size": DefaultProcessMailboxSize, // size of channel for regular messages
	}
	pp := node.Spawn("testGenServer", process_opts, g)
	time.Sleep(1 *time.Second)
	pp.Stop()
	time.Sleep(1 *time.Second)
	node.Stop()
	time.Sleep(1 *time.Second)


}
