package main

import (
	"github.com/ergo-services/ergo/etf"
)

var (
	data = map[string]dataValueSerial{
		"key0": dataValueSerial{"value0", 0},
		"key1": dataValueSerial{"value1", 1},
		"key2": dataValueSerial{"value2", 2},
		"key3": dataValueSerial{"value3", 3},
		"key4": dataValueSerial{"value4", 4},
		"key5": dataValueSerial{"value5", 5},
		"key6": dataValueSerial{"value6", 6},
		"key7": dataValueSerial{"value7", 7},
		"key8": dataValueSerial{"value8", 8},
		"key9": dataValueSerial{"value9", 9},
	}
	keySerials = []string{
		"key0",
		"key1",
		"key2",
		"key3",
		"key4",
		"key5",
		"key6",
		"key7",
		"key8",
		"key9",
	}
)

type storage struct {
	data map[string]dataValueSerial // key/value
	idx  [10]string                 // keys
}

type dataValueSerial struct {
	value  string
	serial uint64
}

func (s *storage) Init(n int) {
	s.data = make(map[string]dataValueSerial)
	for i := 0; i <= int(n); i++ {
		key := keySerials[i]
		s.data[key] = data[key]
		s.idx[i] = key
	}
}

func (s *storage) Read(serial uint64) (string, etf.Term) {
	var key string

	key = s.idx[int(serial)]
	if key == "" {
		// no data found
		return key, nil
	}
	data := s.data[key]
	return key, data.value
}

func (s *storage) Append(serial uint64, key string, value etf.Term) {
	s.data[key] = dataValueSerial{
		value:  value.(string),
		serial: serial,
	}
	s.idx[serial] = key
}
