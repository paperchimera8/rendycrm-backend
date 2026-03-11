package app

import (
	"errors"
	"strings"
	"unicode"
)

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
