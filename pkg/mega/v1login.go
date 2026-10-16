package mega

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/binary"
	"strings"
)

// MEGA v1 login material, mirroring t3rm1n4l/go-mega's password_key and
// stringhash (MIT licensed). Accounts registered by megatools are v1: their
// password key comes from a legacy 65536-round AES KDF and the login user
// handle is a string hash keyed with it. Both are deterministic functions of
// (email, password), so they are precomputed once at export time and embedded
// in the Cloudflare Worker bundle instead of being recomputed on every cold
// isolate (the JS equivalent would take seconds of CPU).

func v1PadTo4(b []byte) []byte {
	if rem := len(b) % 4; rem != 0 {
		b = append(b, make([]byte, 4-rem)...)
	}
	return b
}

func v1PasswordKey(p string) []byte {
	raw := v1PadTo4([]byte(p))
	nw := len(raw) / 4
	words := make([]uint32, nw)
	for i := range words {
		words[i] = binary.BigEndian.Uint32(raw[4*i:])
	}

	pkey := []byte{0x93, 0xC4, 0x67, 0xE3, 0x7D, 0xB0, 0xC7, 0xA4, 0xD1, 0xBE, 0x3F, 0x81, 0x01, 0x52, 0xCB, 0x56}

	n := (nw + 3) / 4
	blocks := make([]cipher.Block, n)
	for j := 0; j < n; j++ {
		key := make([]byte, 16)
		for k := 0; k < 4; k++ {
			idx := 4*j + k
			if idx < nw {
				binary.BigEndian.PutUint32(key[4*k:], words[idx])
			}
		}
		blk, err := aes.NewCipher(key)
		if err != nil {
			// 16-byte keys are always valid; unreachable in practice
			continue
		}
		blocks[j] = blk
	}

	for i := 0; i < 65536; i++ {
		for j := 0; j < n; j++ {
			if blocks[j] != nil {
				blocks[j].Encrypt(pkey, pkey)
			}
		}
	}
	return pkey
}

func v1StringHash(s string, passKey []byte) string {
	raw := v1PadTo4([]byte(s))
	h := []uint32{0, 0, 0, 0}
	for i := 0; i < len(raw); i += 4 {
		h[(i/4)&3] ^= binary.BigEndian.Uint32(raw[i:])
	}

	hb := make([]byte, 16)
	for i, v := range h {
		binary.BigEndian.PutUint32(hb[4*i:], v)
	}

	blk, err := aes.NewCipher(passKey)
	if err != nil {
		return ""
	}
	for i := 0; i < 16384; i++ {
		blk.Encrypt(hb, hb)
	}

	out := make([]byte, 8)
	binary.BigEndian.PutUint32(out[0:], binary.BigEndian.Uint32(hb[0:4]))
	binary.BigEndian.PutUint32(out[4:], binary.BigEndian.Uint32(hb[8:12]))
	return base64.RawURLEncoding.EncodeToString(out)
}

// V1LoginMaterial returns the precomputed MEGA v1 login credentials for an
// account: the base64url-encoded password key and the user handle hash.
func V1LoginMaterial(email, password string) (keyB64, uh string) {
	email = strings.ToLower(strings.TrimSpace(email))
	key := v1PasswordKey(password)
	return base64.RawURLEncoding.EncodeToString(key), v1StringHash(email, key)
}
