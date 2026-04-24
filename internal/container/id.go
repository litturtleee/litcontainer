package container

import (
	"crypto/rand"
	"fmt"
	"io"
)

func generateRandomContainerID() string {
	bytes := make([]byte, 32) // 64个十六进制字符
	if _, err := io.ReadFull(rand.Reader, bytes); err != nil {
		return ""
	}
	return fmt.Sprintf("%x", bytes)
}
