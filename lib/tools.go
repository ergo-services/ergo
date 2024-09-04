package lib

import (
	"crypto/rand"
	"encoding/hex"
)

// RandomString
func RandomString(length int) string {
	buff := make([]byte, length/2)
	rand.Read(buff)
	return hex.EncodeToString(buff)
}
