package usbkey

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Verifier checks for the presence and validity of a USB key
type Verifier struct {
	keyGlob  string
	hashPath string
}

// New creates a new USB key verifier
func New(keyGlob, hashPath string) *Verifier {
	return &Verifier{
		keyGlob:  keyGlob,
		hashPath: hashPath,
	}
}

// Verify checks if a valid USB key is present
// Returns an error if the key is not found or doesn't match the expected hash
func (v *Verifier) Verify() error {
	// Read the expected hash
	expectedHash, err := v.readExpectedHash()
	if err != nil {
		return fmt.Errorf("cannot read expected token hash: %w", err)
	}

	// Find the key file
	keyFile, err := v.findKeyFile()
	if err != nil {
		return fmt.Errorf("USB key not found: %w", err)
	}

	// Verify the key file matches
	ok, err := v.verifyKeyFile(keyFile, expectedHash)
	if err != nil {
		return fmt.Errorf("error verifying USB key: %w", err)
	}
	if !ok {
		return fmt.Errorf("USB key does not match expected token")
	}

	return nil
}

// readExpectedHash reads the expected SHA256 hash from the hash file
func (v *Verifier) readExpectedHash() (string, error) {
	f, err := os.Open(v.hashPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		return "", fmt.Errorf("empty token hash file")
	}

	line := sc.Text()
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return "", fmt.Errorf("invalid token hash file")
	}

	// sha256sum format: "<hash>  <filename>"
	// We just need the hash part
	return strings.ToLower(fields[0]), nil
}

// findKeyFile finds the USB key file using the configured glob pattern
func (v *Verifier) findKeyFile() (string, error) {
	matches, err := filepath.Glob(v.keyGlob)
	if err != nil {
		return "", err
	}

	if len(matches) == 0 {
		return "", fmt.Errorf("no key file matching %q found", v.keyGlob)
	}

	// If multiple matches, use the first one
	// In practice, there should only be one USB key mounted
	return matches[0], nil
}

// verifyKeyFile computes the SHA256 hash of the key file and compares it
func (v *Verifier) verifyKeyFile(path string, expectedHash string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false, err
	}

	actualHash := hex.EncodeToString(h.Sum(nil))
	return actualHash == expectedHash, nil
}
