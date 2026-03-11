package repository

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
)

const demoWorkspaceID = "ws_demo"

func newID(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, time.Now().UTC().UnixNano())
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func normalizeMasterPhone(raw string) (string, error) {
	var digits strings.Builder
	for _, char := range raw {
		if unicode.IsDigit(char) {
			digits.WriteRune(char)
		}
	}
	value := digits.String()
	switch {
	case len(value) == 10:
		value = "7" + value
	case len(value) == 11 && strings.HasPrefix(value, "8"):
		value = "7" + value[1:]
	case len(value) == 11 && strings.HasPrefix(value, "7"):
	default:
		return "", errors.New("номер мастера должен содержать 10 или 11 цифр")
	}
	return value, nil
}
