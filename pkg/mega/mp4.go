package mega

import (
	"encoding/binary"
	"errors"
	"io"
)

// ParseMP4Duration parses the duration in seconds from an MP4 stream by inspecting the moov/mvhd atom.
func ParseMP4Duration(r io.Reader) (float64, error) {
	buf := make([]byte, 8)
	for {
		_, err := io.ReadFull(r, buf)
		if err != nil {
			return 0, err
		}
		boxSize := int64(binary.BigEndian.Uint32(buf[0:4]))
		boxType := string(buf[4:8])

		var payloadSize int64
		if boxSize == 1 {
			ext := make([]byte, 8)
			if _, err := io.ReadFull(r, ext); err != nil {
				return 0, err
			}
			boxSize = int64(binary.BigEndian.Uint64(ext))
			payloadSize = boxSize - 16
		} else if boxSize == 0 {
			payloadSize = 10 * 1024 * 1024
		} else {
			payloadSize = boxSize - 8
		}

		if payloadSize < 0 {
			return 0, errors.New("invalid MP4 box size")
		}

		if boxType == "moov" {
			limitReader := io.LimitReader(r, payloadSize)
			return parseMoovBox(limitReader)
		}

		// Skip unneeded boxes
		if _, err := io.CopyN(io.Discard, r, payloadSize); err != nil {
			return 0, err
		}
	}
}

func parseMoovBox(r io.Reader) (float64, error) {
	buf := make([]byte, 8)
	for {
		_, err := io.ReadFull(r, buf)
		if err != nil {
			return 0, err
		}
		boxSize := int64(binary.BigEndian.Uint32(buf[0:4]))
		boxType := string(buf[4:8])

		payloadSize := boxSize - 8
		if payloadSize < 0 {
			return 0, errors.New("invalid sub-box size in moov")
		}

		if boxType == "mvhd" {
			mvhdData := make([]byte, payloadSize)
			if _, err := io.ReadFull(r, mvhdData); err != nil {
				return 0, err
			}
			if len(mvhdData) < 20 {
				return 0, errors.New("mvhd atom too short")
			}
			version := mvhdData[0]
			if version == 0 {
				if len(mvhdData) < 20 {
					return 0, errors.New("truncated mvhd v0")
				}
				timescale := binary.BigEndian.Uint32(mvhdData[12:16])
				duration := binary.BigEndian.Uint32(mvhdData[16:20])
				if timescale == 0 {
					return 0, errors.New("zero timescale in mvhd")
				}
				return float64(duration) / float64(timescale), nil
			} else if version == 1 {
				if len(mvhdData) < 32 {
					return 0, errors.New("truncated mvhd v1")
				}
				timescale := binary.BigEndian.Uint32(mvhdData[20:24])
				duration := binary.BigEndian.Uint64(mvhdData[24:32])
				if timescale == 0 {
					return 0, errors.New("zero timescale in mvhd")
				}
				return float64(duration) / float64(timescale), nil
			}
			return 0, errors.New("unsupported mvhd version")
		}

		if _, err := io.CopyN(io.Discard, r, payloadSize); err != nil {
			return 0, err
		}
	}
}
