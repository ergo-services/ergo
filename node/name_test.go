package node

import (
	"testing"

	"ergo.services/ergo/gen"
)

func TestValidNodeName(t *testing.T) {
	valid := []gen.Atom{
		"node@localhost",
		"shop-catalog-indexer-7d9f8b5c4-x2m4q@localhost",
		"node@host.example.com",
		"node_1.a@127.0.0.1",
		"моянода@localhost",
		"节点@localhost",
		"a@b",
	}
	for _, name := range valid {
		if err := validNodeName(name); err != nil {
			t.Errorf("%s was refused: %s", name, err)
		}
	}

	refused := map[string]gen.Atom{
		"no at":            "node",
		"two ats":          "a@b@c",
		"empty name":       "@localhost",
		"empty host":       "node@",
		"colon":            "node:1@localhost",
		"slash":            "a/b@localhost",
		"question":         "a?b@localhost",
		"hash":             "a#b@localhost",
		"percent":          "a%20b@localhost",
		"space":            "a b@localhost",
		"newline":          "a\nb@localhost",
		"nbsp":             "a b@localhost",
		"zero width space": "a​b@localhost",
	}
	for what, name := range refused {
		if err := validNodeName(name); err == nil {
			t.Errorf("%s (%q) was accepted", what, string(name))
		}
	}
}

func TestValidNodeNameLeavesTheHostAlone(t *testing.T) {
	for _, name := range []gen.Atom{"node@host:1234", "node@[::1]", "node@under_score"} {
		if err := validNodeName(name); err != nil {
			t.Errorf("%s was refused by the name check: %s", name, err)
		}
	}
}
