package main

import (
	"encoding/binary"
	"fmt"
	"os"
)

func main() {
	buf := make([]byte, 1024)
	n, err := os.Stdin.Read(buf)

	if err != nil {
		return
	}

	if n < 4 {
		fmt.Fprintf(os.Stderr, "incorrect len, %d < 4\n", n)
		return
	}

	l := int(binary.BigEndian.Uint32(buf[:4]))
	if n-4 != l {
		fmt.Fprintf(os.Stderr, "incorrect len (exp: %d, got %d)\n", l, n-4)
		return
	}

	s := fmt.Sprintf("%s pong", string(buf[4:l+4]))
	copy(buf[4:], s)

	ls := len(s)
	binary.BigEndian.PutUint32(buf[:4], uint32(ls))

	os.Stdout.Write(buf[:ls+4])
}
