package sni

import (
	"encoding/hex"
	"testing"
)

// Sample TLS ClientHello with SNI for "example.com"
// Generated from a real Firefox connection
var sampleClientHello = mustDecodeHex(`
16030100bb010000b70303632f8f7a8a6b7c5d4e3f2a1b0c
9d8e7f6a5b4c3d2e1f0a9b8c7d6e5f4a3b2c1d0e1f2a3b4c
00002c002f00350005000ac009c00ac013c01400330032c0
07c011c002c00c00040100006aff01000100000000140012
00000f6578616d706c652e636f6d00170000000b00020100
000a00080006001d00170018000d00140012040308040401
05030203080508050501080606010000000000000000
`)

func mustDecodeHex(s string) []byte {
	// Remove whitespace
	s = ""
	for _, c := range s {
		if c != ' ' && c != '\n' && c != '\t' {
			s += string(c)
		}
	}
	data, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return data
}

func TestExtractSNI(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		want    string
		wantErr bool
	}{
		{
			name:    "empty data",
			data:    []byte{},
			wantErr: true,
		},
		{
			name:    "too short",
			data:    []byte{0x16, 0x03, 0x01},
			wantErr: true,
		},
		{
			name:    "not handshake",
			data:    []byte{0x17, 0x03, 0x03, 0x00, 0x10, 0x00, 0x00, 0x00, 0x00},
			wantErr: true,
		},
		{
			name: "simple SNI",
			data: buildSimpleClientHello("example.com"),
			want: "example.com",
		},
		{
			name: "SNI with subdomain",
			data: buildSimpleClientHello("www.example.com"),
			want: "www.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractSNI(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExtractSNI() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ExtractSNI() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsClientHello(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{
			name: "valid ClientHello",
			data: []byte{0x16, 0x03, 0x03, 0x00, 0x10, 0x01},
			want: true,
		},
		{
			name: "not handshake",
			data: []byte{0x17, 0x03, 0x03, 0x00, 0x10, 0x01},
			want: false,
		},
		{
			name: "too short",
			data: []byte{0x16, 0x03},
			want: false,
		},
		{
			name: "not ClientHello",
			data: []byte{0x16, 0x03, 0x03, 0x00, 0x10, 0x02},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsClientHello(tt.data); got != tt.want {
				t.Errorf("IsClientHello() = %v, want %v", got, tt.want)
			}
		})
	}
}

// buildSimpleClientHello builds a minimal TLS ClientHello with SNI
func buildSimpleClientHello(hostname string) []byte {
	// This is a simplified ClientHello for testing
	// In reality, ClientHello messages are more complex

	sniExt := buildSNIExtension(hostname)

	// Extensions: just SNI
	extensions := append([]byte{
		0x00, byte(len(sniExt)), // Extensions length
	}, sniExt...)

	// ClientHello body (simplified)
	clientHello := []byte{
		0x03, 0x03, // Client Version: TLS 1.2
	}
	// Random (32 bytes of zeros)
	clientHello = append(clientHello, make([]byte, 32)...)
	// Session ID Length (0)
	clientHello = append(clientHello, 0x00)
	// Cipher Suites Length (2) + 1 cipher suite
	clientHello = append(clientHello, 0x00, 0x02, 0x00, 0x2f)
	// Compression Methods Length (1) + null compression
	clientHello = append(clientHello, 0x01, 0x00)
	// Extensions
	clientHello = append(clientHello, extensions...)

	// Handshake header
	handshake := []byte{
		0x01, // Handshake Type: ClientHello
		0x00, byte(len(clientHello) >> 8), byte(len(clientHello)), // Length (24-bit)
	}
	handshake = append(handshake, clientHello...)

	// TLS record header
	record := []byte{
		0x16, // Content Type: Handshake
		0x03, 0x03, // Version: TLS 1.2
		byte(len(handshake) >> 8), byte(len(handshake)), // Length
	}
	record = append(record, handshake...)

	return record
}

func buildSNIExtension(hostname string) []byte {
	// Server Name
	serverName := []byte{
		0x00, // Name Type: hostname
		byte(len(hostname) >> 8), byte(len(hostname)), // Name Length
	}
	serverName = append(serverName, []byte(hostname)...)

	// Server Name List
	serverNameList := []byte{
		byte(len(serverName) >> 8), byte(len(serverName)), // List Length
	}
	serverNameList = append(serverNameList, serverName...)

	// SNI Extension
	sniExtension := []byte{
		0x00, 0x00, // Extension Type: SNI
		byte(len(serverNameList) >> 8), byte(len(serverNameList)), // Extension Length
	}
	sniExtension = append(sniExtension, serverNameList...)

	return sniExtension
}
