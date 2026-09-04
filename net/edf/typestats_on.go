//go:build typestats

package edf

import (
	"fmt"
	"reflect"
	"sync/atomic"

	"ergo.services/ergo/lib"
)

// statsEnabled is set to true under -tags=typestats. Used at type
// registration to mark RegisteredTypeStats.Enabled.
const statsEnabled = true

// encodeWithStats wraps enc.Encode and records the operation when
// enc.Info is populated. Composite ad-hoc encoders without Info skip
// the stats path entirely.
func encodeWithStats(enc *encoder, xv reflect.Value, b *lib.Buffer, state *stateEncode) error {
	if enc.Info == nil {
		return enc.Encode(xv, b, state)
	}
	startLen := b.Len()
	err := enc.Encode(xv, b, state)
	if err != nil {
		return err
	}
	atomic.AddInt64(&enc.Info.Stats.Encoded, 1)
	atomic.AddInt64(&enc.Info.Stats.EncodedBytes, int64(b.Len()-startLen))
	return nil
}

// decodeWithStats performs full root-level decoding (getDecoder + dec.Decode +
// .Interface() conversion) and records the operation. startLen is captured
// before getDecoder so that DecodedBytes includes the type-prefix bytes
// consumed by getDecoder, matching how EncodedBytes includes the prefix.
func decodeWithStats(packet []byte, state *stateDecode) (any, []byte, error) {
	startLen := len(packet)

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

	if dec.Info != nil {
		atomic.AddInt64(&dec.Info.Stats.Decoded, 1)
		atomic.AddInt64(&dec.Info.Stats.DecodedBytes, int64(startLen-len(packet)))
	}

	if value == nil {
		return v.Interface(), packet, nil
	}
	return value.Interface(), packet, nil
}
