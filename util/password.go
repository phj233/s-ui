package util

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	passwordHashMemory      = 19 * 1024
	passwordHashIterations  = 2
	passwordHashParallelism = 1
	passwordHashSaltLength  = 16
	passwordHashKeyLength   = 32
)

func HashPassword(password string) (string, error) {
	salt := make([]byte, passwordHashSaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}

	hash := argon2.IDKey(
		[]byte(password),
		salt,
		passwordHashIterations,
		passwordHashMemory,
		passwordHashParallelism,
		passwordHashKeyLength,
	)

	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		passwordHashMemory,
		passwordHashIterations,
		passwordHashParallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

func IsPasswordHash(password string) bool {
	_, _, _, _, _, err := parsePasswordHash(password)
	return err == nil
}

func PasswordMatches(stored string, password string) bool {
	memory, iterations, parallelism, salt, hash, err := parsePasswordHash(stored)
	if err != nil {
		return subtle.ConstantTimeCompare([]byte(stored), []byte(password)) == 1
	}

	otherHash := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, uint32(len(hash)))
	return subtle.ConstantTimeCompare(hash, otherHash) == 1
}

func parsePasswordHash(encoded string) (uint32, uint32, uint8, []byte, []byte, error) {
	vals := strings.Split(encoded, "$")
	if len(vals) != 6 || vals[1] != "argon2id" {
		return 0, 0, 0, nil, nil, fmt.Errorf("invalid password hash")
	}

	var version int
	if _, err := fmt.Sscanf(vals[2], "v=%d", &version); err != nil {
		return 0, 0, 0, nil, nil, err
	}
	if version != argon2.Version {
		return 0, 0, 0, nil, nil, fmt.Errorf("unsupported argon2 version")
	}

	var memory uint32
	var iterations uint32
	var parallelism uint8
	if _, err := fmt.Sscanf(vals[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism); err != nil {
		return 0, 0, 0, nil, nil, err
	}

	salt, err := base64.RawStdEncoding.DecodeString(vals[4])
	if err != nil {
		return 0, 0, 0, nil, nil, err
	}
	hash, err := base64.RawStdEncoding.DecodeString(vals[5])
	if err != nil {
		return 0, 0, 0, nil, nil, err
	}
	if memory == 0 || iterations == 0 || parallelism == 0 || len(salt) == 0 || len(hash) == 0 {
		return 0, 0, 0, nil, nil, fmt.Errorf("invalid password hash parameters")
	}

	return memory, iterations, parallelism, salt, hash, nil
}
