package system

import (
	"encoding/binary"
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"
)

func TestNormalizeCommandOutputKeepsUTF8(t *testing.T) {
	const value = "浏览器及依赖包"
	if got := normalizeCommandOutput([]byte(value)); got != value {
		t.Fatalf("UTF-8 output = %q, want %q", got, value)
	}
}

func TestNormalizeCommandOutputDecodesGB18030(t *testing.T) {
	const value = "浏览器及依赖包"
	encoded, err := simplifiedchinese.GB18030.NewEncoder().Bytes([]byte(value))
	if err != nil {
		t.Fatal(err)
	}
	if got := normalizeCommandOutput(encoded); got != value {
		t.Fatalf("GB18030 output = %q, want %q", got, value)
	}
}

func TestNormalizeCommandOutputDecodesUTF16LE(t *testing.T) {
	const value = "浏览器"
	encoded := []byte{0xff, 0xfe}
	for _, unit := range []uint16{0x6d4f, 0x89c8, 0x5668} {
		var bytes [2]byte
		binary.LittleEndian.PutUint16(bytes[:], unit)
		encoded = append(encoded, bytes[:]...)
	}
	if got := normalizeCommandOutput(encoded); got != value {
		t.Fatalf("UTF-16LE output = %q, want %q", got, value)
	}
}
