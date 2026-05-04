package lib

import (
	"fmt"
	"io"
	"os"
	"sync"
)

var (
	deprecationMu      sync.Mutex
	deprecationEmitted = make(map[string]struct{})
)

// EmitDeprecation writes a deprecation warning at most once per name.
// If w is nil, os.Stderr is used.
func EmitDeprecation(w io.Writer, name, replacement, url string) {
	deprecationMu.Lock()
	if _, exists := deprecationEmitted[name]; exists {
		deprecationMu.Unlock()
		return
	}
	deprecationEmitted[name] = struct{}{}
	deprecationMu.Unlock()

	if w == nil {
		w = os.Stderr
	}
	fmt.Fprintf(w, "[ergo] DEPRECATED: %s. Use %s instead. See %s\n",
		name, replacement, url)
}
