package common

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

func GenerateHMACWithKey(key []byte, data string) string {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}

func GenerateHMAC(data string) string {
	h := hmac.New(sha256.New, []byte(CryptoSecret))
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}

func Password2Hash(password string) (string, error) {
	return Password2HashContext(context.Background(), password)
}

func Password2HashContext(ctx context.Context, password string) (string, error) {
	release, err := acquirePasswordKDF(ctx)
	if err != nil {
		return "", err
	}
	defer release()
	passwordBytes := []byte(password)
	hashedPassword, err := bcrypt.GenerateFromPassword(passwordBytes, bcrypt.DefaultCost)
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	return string(hashedPassword), err
}

func ValidatePasswordAndHash(password string, hash string) bool {
	valid, _ := ValidatePasswordAndHashContext(context.Background(), password, hash)
	return valid
}

func ValidatePasswordAndHashContext(ctx context.Context, password string, hash string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if strings.HasPrefix(hash, "$argon2id$") {
		return validateArgon2AccountPasswordContext(ctx, password, hash)
	}
	release, err := acquirePasswordKDF(ctx)
	if err != nil {
		return false, err
	}
	defer release()
	err = bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	return err == nil, nil
}
