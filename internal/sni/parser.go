package sni

import (
	"encoding/binary"
	"errors"
)

// TLS constants
const (
	contentTypeHandshake = 0x16
	handshakeTypeClientHello = 0x01
	extensionTypeSNI = 0x0000
	sniNameTypeHostname = 0x00
)

var (
	ErrNotHandshake = errors.New("not a TLS handshake record")
	ErrNotClientHello = errors.New("not a ClientHello message")
	ErrNoSNI = errors.New("no SNI extension found")
	ErrInvalidData = errors.New("invalid TLS data")
)

// ExtractSNI extracts the Server Name Indication from a TLS ClientHello message.
// It parses the TLS record without decryption, reading the plaintext ClientHello.
func ExtractSNI(data []byte) (string, error) {
	// Need at least 5 bytes for TLS record header
	if len(data) < 5 {
		return "", ErrInvalidData
	}

	// Parse TLS Record Header (5 bytes)
	// Byte 0: Content Type
	// Bytes 1-2: TLS Version
	// Bytes 3-4: Record Length
	contentType := data[0]
	recordLength := binary.BigEndian.Uint16(data[3:5])

	// Check if this is a handshake record
	if contentType != contentTypeHandshake {
		return "", ErrNotHandshake
	}

	// Check if we have enough data for the full record
	if len(data) < int(5+recordLength) {
		return "", ErrInvalidData
	}

	// Parse Handshake Header (4 bytes)
	// Byte 5: Handshake Type
	// Bytes 6-8: Handshake Length (24-bit)
	if len(data) < 9 {
		return "", ErrInvalidData
	}

	handshakeType := data[5]
	if handshakeType != handshakeTypeClientHello {
		return "", ErrNotClientHello
	}

	// Start parsing ClientHello
	pos := 9 // After TLS record + handshake headers

	// Client Version (2 bytes)
	pos += 2

	// Random (32 bytes)
	pos += 32

	if pos >= len(data) {
		return "", ErrInvalidData
	}

	// Session ID Length (1 byte) + Session ID
	sessionIDLength := int(data[pos])
	pos += 1 + sessionIDLength

	if pos+2 > len(data) {
		return "", ErrInvalidData
	}

	// Cipher Suites Length (2 bytes) + Cipher Suites
	cipherSuitesLength := int(binary.BigEndian.Uint16(data[pos : pos+2]))
	pos += 2 + cipherSuitesLength

	if pos >= len(data) {
		return "", ErrInvalidData
	}

	// Compression Methods Length (1 byte) + Compression Methods
	compressionMethodsLength := int(data[pos])
	pos += 1 + compressionMethodsLength

	if pos+2 > len(data) {
		return "", ErrInvalidData
	}

	// Extensions Length (2 bytes)
	extensionsLength := int(binary.BigEndian.Uint16(data[pos : pos+2]))
	pos += 2

	// Parse Extensions
	extensionsEnd := pos + extensionsLength
	for pos+4 <= extensionsEnd {
		// Extension Type (2 bytes) + Extension Length (2 bytes)
		extType := binary.BigEndian.Uint16(data[pos : pos+2])
		extLength := int(binary.BigEndian.Uint16(data[pos+2 : pos+4]))
		pos += 4

		if pos+extLength > len(data) {
			return "", ErrInvalidData
		}

		// Check if this is SNI extension
		if extType == extensionTypeSNI {
			return parseSNIExtension(data[pos : pos+extLength])
		}

		pos += extLength
	}

	return "", ErrNoSNI
}

// parseSNIExtension parses the SNI extension data to extract the hostname.
//
// SNI extension format:
// - Server Name List Length (2 bytes)
// - Server Name Type (1 byte, 0x00 = hostname)
// - Server Name Length (2 bytes)
// - Server Name (variable, ASCII hostname)
func parseSNIExtension(data []byte) (string, error) {
	if len(data) < 5 {
		return "", ErrInvalidData
	}

	// Server Name List Length (2 bytes) - skip it
	_ = int(binary.BigEndian.Uint16(data[0:2]))
	pos := 2

	if pos >= len(data) {
		return "", ErrInvalidData
	}

	// We only care about the first name
	nameType := data[pos]
	pos++

	// Check if it's a hostname
	if nameType != sniNameTypeHostname {
		return "", ErrNoSNI
	}

	if pos+2 > len(data) {
		return "", ErrInvalidData
	}

	// Server Name Length (2 bytes)
	nameLength := int(binary.BigEndian.Uint16(data[pos : pos+2]))
	pos += 2

	if pos+nameLength > len(data) {
		return "", ErrInvalidData
	}

	// Extract hostname (ASCII)
	hostname := string(data[pos : pos+nameLength])

	if hostname == "" {
		return "", ErrNoSNI
	}

	return hostname, nil
}

// IsClientHello performs a quick check if data looks like a TLS ClientHello.
// This can be used as a fast pre-filter before calling ExtractSNI.
func IsClientHello(data []byte) bool {
	if len(data) < 6 {
		return false
	}

	// Check TLS record header
	if data[0] != contentTypeHandshake {
		return false
	}

	// Check handshake type
	if data[5] != handshakeTypeClientHello {
		return false
	}

	return true
}
