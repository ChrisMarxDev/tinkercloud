package webui

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
)

func TestQRCodeMatrixIsLocalBoundedAndMatchesVerifiedSymbol(t *testing.T) {
	const target = "https://payroll-preview.apps.example.test/"
	encoded := qrCodeData(target)
	if encoded == "" || strings.Contains(encoded, target) {
		t.Fatalf("QR matrix was empty or exposed its source URL directly: %q", encoded)
	}
	packed, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode QR matrix: %v", err)
	}
	if got, want := len(packed), (qrSize*qrSize+7)/8; got != want {
		t.Fatalf("packed matrix bytes = %d, want %d", got, want)
	}
	if got := qrCodeData(strings.Repeat("x", qrMaxBytes+1)); got != "" {
		t.Fatalf("oversized input rendered a broken QR code: %s", got)
	}

	modules := qrModules([]byte(target))
	flat := make([]byte, 0, qrSize*qrSize)
	for y, row := range modules {
		for x, dark := range row {
			if dark {
				flat = append(flat, '1')
			} else {
				flat = append(flat, '0')
			}
			index := y*qrSize + x
			packedDark := packed[index>>3]&(1<<uint(7-(index&7))) != 0
			if packedDark != dark {
				t.Fatalf("packed matrix differs at (%d,%d)", x, y)
			}
		}
	}
	if got, want := fmt.Sprintf("%x", sha256.Sum256(flat)), "9e8d630715aea073e7a8cba344cf2f7604b575339c7bff7d50ab169ff07245c0"; got != want {
		t.Fatalf("verified QR matrix hash = %s, want %s", got, want)
	}
	if len(modules) != qrSize {
		t.Fatalf("QR size = %d, want %d", len(modules), qrSize)
	}
	for _, row := range modules {
		if len(row) != qrSize {
			t.Fatalf("QR row size = %d, want %d", len(row), qrSize)
		}
	}
	for _, point := range [][2]int{{0, 0}, {6, 0}, {0, 6}, {qrSize - 1, 0}, {0, qrSize - 1}} {
		if !modules[point[1]][point[0]] {
			t.Fatalf("finder pattern corner (%d,%d) is not dark", point[0], point[1])
		}
	}
	if got := len(qrCodewords([]byte(target))); got != 346 {
		t.Fatalf("codeword count = %d, want 346", got)
	}
}
