package node

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

var rawB64 = base64.RawURLEncoding

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func randomNonce(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return rawB64.EncodeToString(b), nil
}

func decodeJSONLimited(r io.Reader, max int64, v any) error {
	lr := io.LimitReader(r, max+1)
	dec := json.NewDecoder(lr)
	if err := dec.Decode(v); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}

func sanitizeMessage(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(s, "\r", " "), "\n", " "))
	if len(s) > 500 {
		s = s[:500]
	}
	return s
}
