package mega

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"time"

	gomega "github.com/t3rm1n4l/go-mega"
)

type apiNode struct {
	Hash   string `json:"h"`
	Parent string `json:"p"`
	User   string `json:"u"`
	Type   int    `json:"t"`
	Key    string `json:"k"`
}

type apiShareKey struct {
	Hash string `json:"h"`
	Key  string `json:"k"`
	Ha   string `json:"ha"`
}

type apiFilesResp struct {
	F  []apiNode     `json:"f"`
	Ok []apiShareKey `json:"ok"`
}

type s2ShareUser struct {
	User string `json:"u"`
	Role int    `json:"r"`
}

type s2Payload struct {
	Cmd string        `json:"a"`
	N   string        `json:"n"`
	S   []s2ShareUser `json:"s"`
	Ok  string        `json:"ok"`
	Ha  string        `json:"ha"`
	Cr  []any         `json:"cr"`
}

type lPayload struct {
	Cmd string `json:"a"`
	N   string `json:"n"`
}

type fPayload struct {
	Cmd string `json:"a"`
	C   int    `json:"c"`
}

func encryptECB(block cipher.Block, dst, src []byte) {
	for len(src) >= 16 && len(dst) >= 16 {
		block.Encrypt(dst[:16], src[:16])
		src = src[16:]
		dst = dst[16:]
	}
}

func decryptECB(block cipher.Block, dst, src []byte) {
	for len(src) >= 16 && len(dst) >= 16 {
		block.Decrypt(dst[:16], src[:16])
		src = src[16:]
		dst = dst[16:]
	}
}

func callMegaAPI(sid string, payload any) ([]byte, error) {
	sn, _ := rand.Int(rand.Reader, big.NewInt(2147483647))
	apiURL := fmt.Sprintf("https://g.api.mega.co.nz/cs?id=%d", sn.Int64())
	if sid != "" {
		apiURL = fmt.Sprintf("%s&sid=%s", apiURL, sid)
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("json marshal error: %w", err)
	}

	req, err := http.NewRequest("POST", apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("http request error: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("api request failed: %w", err)
	}
	defer resp.Body.Close()

	resBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var intArr []int
	if err := json.Unmarshal(resBytes, &intArr); err == nil && len(intArr) > 0 {
		if intArr[0] < 0 {
			return nil, fmt.Errorf("mega error code %d", intArr[0])
		}
	}

	var singleInt int
	if err := json.Unmarshal(resBytes, &singleInt); err == nil && singleInt < 0 {
		return nil, fmt.Errorf("mega error code %d", singleInt)
	}

	return resBytes, nil
}

// ExportFolderViaS2 performs the robust 2-step MEGA folder public link handshake
func ExportFolderViaS2(m *gomega.Mega, folderNode *gomega.Node) (string, string, string, error) {
	if folderNode == nil {
		return "", "", "", fmt.Errorf("folder node is nil")
	}

	folderHandle := folderNode.GetHash()
	if folderHandle == "" {
		return "", "", "", fmt.Errorf("invalid folder handle")
	}

	masterKey := m.GetMasterKey()
	if len(masterKey) != 16 {
		return "", "", "", fmt.Errorf("invalid master key length: %d", len(masterKey))
	}

	sid := m.GetSessionID()
	if sid == "" {
		return "", "", "", fmt.Errorf("empty session id")
	}

	masterCipher, err := aes.NewCipher(masterKey)
	if err != nil {
		return "", "", "", fmt.Errorf("aes cipher error: %w", err)
	}

	fReq := []fPayload{{Cmd: "f", C: 1}}
	fResBytes, err := callMegaAPI(sid, fReq)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to fetch filesystem nodes: %w", err)
	}

	var fResp []apiFilesResp
	if err := json.Unmarshal(fResBytes, &fResp); err != nil || len(fResp) == 0 {
		return "", "", "", fmt.Errorf("invalid filesystem response: %s", string(fResBytes))
	}

	var existingShareKey []byte
	for _, sk := range fResp[0].Ok {
		if sk.Hash == folderHandle {
			encBytes, decErr := base64.RawURLEncoding.DecodeString(sk.Key)
			if decErr == nil && len(encBytes) == 16 {
				plain := make([]byte, 16)
				decryptECB(masterCipher, plain, encBytes)
				existingShareKey = plain
				break
			}
		}
	}

	var finalShareKey []byte

	if len(existingShareKey) == 16 {
		finalShareKey = existingShareKey
	} else {
		// Generate 16-byte shareKey
		shareKey := make([]byte, 16)
		if _, err := rand.Read(shareKey); err != nil {
			return "", "", "", fmt.Errorf("failed to generate random share key: %w", err)
		}
		finalShareKey = shareKey

		okBuf := make([]byte, 16)
		encryptECB(masterCipher, okBuf, shareKey)
		okB64 := base64.RawURLEncoding.EncodeToString(okBuf)

		authRaw := []byte(folderHandle + folderHandle)
		if len(authRaw) < 16 {
			authRaw = append(authRaw, make([]byte, 16-len(authRaw))...)
		} else if len(authRaw) > 16 {
			authRaw = authRaw[:16]
		}
		haBuf := make([]byte, 16)
		encryptECB(masterCipher, haBuf, authRaw)
		haB64 := base64.RawURLEncoding.EncodeToString(haBuf)

		byParent := make(map[string][]apiNode)
		for _, node := range fResp[0].F {
			byParent[node.Parent] = append(byParent[node.Parent], node)
		}

		var descendants []apiNode
		var collect func(h string)
		collect = func(h string) {
			for _, ch := range byParent[h] {
				descendants = append(descendants, ch)
				if ch.Type == 1 {
					collect(ch.Hash)
				}
			}
		}
		collect(folderHandle)

		nodeMap := make(map[string]int)
		nodeMap[folderHandle] = 0
		sources := []string{folderHandle}
		for _, d := range descendants {
			nodeMap[d.Hash] = len(sources)
			sources = append(sources, d.Hash)
		}

		shares := []string{folderHandle}
		crKeys := []any{}

		shareCipher, sErr := aes.NewCipher(shareKey)
		if sErr != nil {
			return "", "", "", fmt.Errorf("failed to init share cipher: %w", sErr)
		}

		for _, d := range descendants {
			sourceIdx, exists := nodeMap[d.Hash]
			if !exists || d.Key == "" {
				continue
			}

			kParts := strings.Split(d.Key, ":")
			if len(kParts) < 2 {
				continue
			}
			itemKey := strings.Split(kParts[1], "/")[0]
			encBytes, decErr := base64.RawURLEncoding.DecodeString(itemKey)
			if decErr != nil || (len(encBytes) != 16 && len(encBytes) != 32) {
				continue
			}

			plainKey := make([]byte, len(encBytes))
			decryptECB(masterCipher, plainKey, encBytes)

			reEnc := make([]byte, len(plainKey))
			encryptECB(shareCipher, reEnc, plainKey)

			crKeys = append(crKeys, 0, sourceIdx, base64.RawURLEncoding.EncodeToString(reEnc))
		}

		cr := []any{
			shares,
			sources,
			crKeys,
		}

		s2Req := []s2Payload{
			{
				Cmd: "s2",
				N:   folderHandle,
				S:   []s2ShareUser{{User: "EXP", Role: 0}},
				Ok:  okB64,
				Ha:  haB64,
				Cr:  cr,
			},
		}

		_, s2Err := callMegaAPI(sid, s2Req)
		if s2Err != nil {
			return "", "", "", fmt.Errorf("s2 folder share failed: %w", s2Err)
		}
	}

	// Step 2: Obtain link handle with "l" command
	lReq := []lPayload{{Cmd: "l", N: folderHandle}}
	lResBytes, lErr := callMegaAPI(sid, lReq)
	if lErr != nil {
		return "", "", "", fmt.Errorf("failed to export link handle: %w", lErr)
	}

	var lRes []string
	if err := json.Unmarshal(lResBytes, &lRes); err != nil || len(lRes) == 0 || lRes[0] == "" {
		// Fallback: sometimes l returns an object or int
		var lStr string
		if err2 := json.Unmarshal(lResBytes, &lStr); err2 == nil && lStr != "" {
			lRes = []string{lStr}
		} else {
			return "", "", "", fmt.Errorf("could not parse public link id from response: %s", string(lResBytes))
		}
	}

	linkHandle := lRes[0]

	// MEGA folder keys for public links require the 16-byte share key + 16-byte attributes IV/key mix (total 32 bytes / 43 base64url chars).
	// Let's ensure the 32-byte complete decryption key is encoded if available, or padded to 32 bytes.
	fullKey := make([]byte, 32)
	copy(fullKey[:16], finalShareKey)
	// If folder node has a key, decrypt it and use the extra 16 bytes for full decryption
	for _, n := range fResp[0].F {
		if n.Hash == folderHandle && n.Key != "" {
			kParts := strings.Split(n.Key, ":")
			if len(kParts) >= 2 {
				itemKey := strings.Split(kParts[1], "/")[0]
				if eb, decErr := base64.RawURLEncoding.DecodeString(itemKey); decErr == nil && len(eb) >= 32 {
					plain := make([]byte, len(eb))
					decryptECB(masterCipher, plain, eb)
					copy(fullKey[16:], plain[16:32])
				}
			}
			break
		}
	}

	keyB64 := base64.RawURLEncoding.EncodeToString(fullKey)
	publicURL := fmt.Sprintf("https://mega.nz/folder/%s#%s", linkHandle, keyB64)

	return linkHandle, keyB64, publicURL, nil
}
