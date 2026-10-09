package domain

import (
	"crypto/rand"
	"io"
	"math/big"
	"strings"
)

// CodeAlphabet excludes ambiguous characters (0/O, 1/I/L).
const CodeAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

// CodeLength is the length of a party code.
const CodeLength = 6

// GenerateCode returns a random party code using crypto/rand.
func GenerateCode() (string, error) {
	return generateCode(rand.Reader)
}

func generateCode(r io.Reader) (string, error) {
	max := big.NewInt(int64(len(CodeAlphabet)))
	var b strings.Builder
	b.Grow(CodeLength)
	for i := 0; i < CodeLength; i++ {
		n, err := rand.Int(r, max)
		if err != nil {
			return "", err
		}
		b.WriteByte(CodeAlphabet[n.Int64()])
	}
	return b.String(), nil
}

// NormalizeCode uppercases a user typed code and strips spaces and dashes.
func NormalizeCode(s string) string {
	s = strings.ToUpper(s)
	return strings.Map(func(r rune) rune {
		if r == ' ' || r == '-' || r == '\t' {
			return -1
		}
		return r
	}, s)
}

// IsValidCode reports whether s is a well formed party code.
func IsValidCode(s string) bool {
	if len(s) != CodeLength {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !strings.ContainsRune(CodeAlphabet, rune(s[i])) {
			return false
		}
	}
	return true
}
