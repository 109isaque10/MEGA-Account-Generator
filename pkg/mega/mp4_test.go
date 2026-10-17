package mega_test

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"

	"github.com/109isaque10/mega-account-generator/pkg/mega"
)

func TestParseMP4Duration(t *testing.T) {
	// Construct a synthetic MP4 with ftyp and moov/mvhd
	var buf bytes.Buffer

	// 1. ftyp box (16 bytes)
	_ = binary.Write(&buf, binary.BigEndian, uint32(16))
	buf.WriteString("ftyp")
	buf.WriteString("isom")
	_ = binary.Write(&buf, binary.BigEndian, uint32(512))

	// 2. mvhd payload (version 0): 24 bytes
	var mvhdPayload bytes.Buffer
	mvhdPayload.WriteByte(0)                    // version
	mvhdPayload.Write([]byte{0, 0, 0})          // flags
	_ = binary.Write(&mvhdPayload, binary.BigEndian, uint32(0)) // creation
	_ = binary.Write(&mvhdPayload, binary.BigEndian, uint32(0)) // modification
	_ = binary.Write(&mvhdPayload, binary.BigEndian, uint32(1000)) // timescale = 1000 Hz
	_ = binary.Write(&mvhdPayload, binary.BigEndian, uint32(120500)) // duration = 120500 units => 120.5s

	// mvhd box
	mvhdBoxLen := uint32(8 + mvhdPayload.Len())
	var mvhdBox bytes.Buffer
	_ = binary.Write(&mvhdBox, binary.BigEndian, mvhdBoxLen)
	mvhdBox.WriteString("mvhd")
	mvhdBox.Write(mvhdPayload.Bytes())

	// moov box
	moovBoxLen := uint32(8 + mvhdBox.Len())
	_ = binary.Write(&buf, binary.BigEndian, moovBoxLen)
	buf.WriteString("moov")
	buf.Write(mvhdBox.Bytes())

	dur, err := mega.ParseMP4Duration(&buf)
	if err != nil {
		t.Fatalf("unexpected error parsing MP4 duration: %v", err)
	}

	expected := 120.5
	if math.Abs(dur-expected) > 0.001 {
		t.Errorf("expected duration %v, got %v", expected, dur)
	}
}
