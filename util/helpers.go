package util

import (
	"crypto/rand"
	"fmt"
)

func RandomHex(size int) (string, error) {
	buf := make([]byte, size)
	_, err := rand.Read(buf)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", buf), nil
}
