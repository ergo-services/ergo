package cloud

import "github.com/ergo-services/ergo/gen"

const (
	EventCloud gen.Event = "cloud"
)

type MessageEventCloud struct {
	Online bool
}
