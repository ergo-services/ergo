package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {

	buf := make([]byte, 1024)
	n, err := os.Stdin.Read(buf)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to read stdin: %s\n", err)
		return
	}
	fmt.Println(strings.TrimRight(string(buf[:n]), "\r\n"), "pong")
}
