//go:build !typestats

package edf

import (
	"fmt"
	"reflect"

	"ergo.services/ergo/lib"
)

// statsEnabled is false without -tags=typestats. Used at type
// registration to leave RegisteredTypeStats.Enabled as zero value.
const statsEnabled = false

// encodeWithStats is a pass-through wrapper without -tags=typestats.
// The Go inliner reduces this to a direct call to enc.Encode.
func encodeWithStats(enc *encoder, xv reflect.Value, b *lib.Buffer, state *stateEncode) error {
	return enc.Encode(xv, b, state)
}

// decodeWithStats performs full root-level decoding without recording stats.
func decodeWithStats(packet []byte, state *stateDecode) (any, []byte, error) {
	dec, packet, err := getDecoder(packet, state)
	if err != nil {
		return nil, nil, err
	}
	if dec == nil {
		return nil, packet, nil
	}

	state.decoder = dec
	v := reflect.Indirect(reflect.New(dec.Type))

	value, packet, err := dec.Decode(&v, packet, state)
	if err != nil {
		return nil, nil, fmt.Errorf("malformed EDF for %s: %w", dec.Type.Name(), err)
	}

	if value == nil {
		return v.Interface(), packet, nil
	}
	return value.Interface(), packet, nil
}
