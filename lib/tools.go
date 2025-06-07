package lib

import (
	"crypto/rand"
	"encoding/hex"
	"os"
)

// RandomString
func RandomString(length int) string {
	buff := make([]byte, length/2)
	rand.Read(buff)
	return hex.EncodeToString(buff)
}

func GetHostname() string {
	// Check if it's running in Kubernetes.
	// Kubernetes is not ideal for stateful services (especially regarding DNS management within a cluster),
	// which is why the pod's IP address has to be used instead of its hostname.
	if podIP := os.Getenv("POD_IP"); podIP != "" {
		return podIP
	}

	// Is it running inside docker? Then use the hostname.
	// Docker creates a .dockerenv file at the root of the directory tree inside the container
	if _, err := os.Stat("/.dockerenv"); err == nil {
		if hostname, err := os.Hostname(); err == nil {
			return hostname
		}
	}

	// Otherwise, use 'localhost' as a hostname
	return "localhost"
}
