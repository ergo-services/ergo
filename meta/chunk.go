package meta

import (
	"fmt"
)

type ChunkOptions struct {
	Enable                     bool
	FixedLength                int
	HeaderSize                 int
	HeaderLengthPosition       int // within the header
	HeaderLengthSize           int // 1, 2 or 4
	HeaderLengthIncludesHeader bool
	MaxLength                  int
}

func (co ChunkOptions) IsValid() error {
	if co.Enable == false {
		return nil
	}

	if co.FixedLength > 0 {
		return nil
	}

	// dynamic length
	if co.HeaderSize == 0 {
		return fmt.Errorf("chunk HeaderSize must be non-zero for dynamic chunk size")
	}

	hl := co.HeaderLengthSize + co.HeaderLengthPosition
	if hl > co.HeaderSize {
		return fmt.Errorf("chunk HeaderLengthPosition + ...LengthSize is out of HeaderSize bounds")
	}

	switch co.HeaderLengthSize {
	case 1, 2, 4:
	default:
		return fmt.Errorf("chunk HeaderLengthSize must be either: 1, 2, or 4 bytes")
	}

	return nil
}
