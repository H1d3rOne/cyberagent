package daemon

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const tokenBytes = 32

func loadOrCreateToken(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("token file is required")
	}
	if data, err := os.ReadFile(path); err == nil {
		token := strings.TrimSpace(string(data))
		if token == "" {
			return "", fmt.Errorf("token file %s is empty", path)
		}
		return token, nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("read token file %s: %w", path, err)
	}

	token, err := generateToken()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", fmt.Errorf("create token directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(token+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("write token file %s: %w", path, err)
	}
	return token, nil
}

func generateToken() (string, error) {
	buf := make([]byte, tokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
