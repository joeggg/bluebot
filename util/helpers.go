package util

import (
	"fmt"
	"math/rand"
	"time"
)

func RandomHex(size int) (string, error) {
	buf := make([]byte, size)
	rand.Seed(time.Now().UnixMicro())
	_, err := rand.Read(buf)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", buf), nil
}
