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

	// Check for top-level error codes like [-11] or -11
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

// ExportFolderViaS2 performs the official 2-step MEGA folder public link handshake:
// 1. "s2" command: registers the folder as an exported share (EXP) with folder share key & cr
// 2. "l" command: obtains the public link ID
// Returns linkHandle, base64url(shareKey), publicURL, error
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

	// Step 1: Query filesystem to see existing share keys and descendant nodes
	fReq := []fPayload{{Cmd: "f", C: 1}}
	fResBytes, err := callMegaAPI(sid, fReq)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to fetch filesystem nodes: %w", err)
	}

	var fResp []apiFilesResp
	if err := json.Unmarshal(fResBytes, &fResp); err != nil || len(fResp) == 0 {
		return "", "", "", fmt.Errorf("invalid filesystem response: %s", string(fResBytes))
	}

	// Check if this folder already has an exported share key in 'ok'
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
		// Folder is not yet shared. Generate a new 16-byte shareKey and send "s2" command
		shareKey := make([]byte, 16)
		if _, err := rand.Read(shareKey); err != nil {
			return "", "", "", fmt.Errorf("failed to generate random share key: %w", err)
		}
		finalShareKey = shareKey

		// ok = AES-ECB(masterKey, shareKey)
		okBuf := make([]byte, 16)
		encryptECB(masterCipher, okBuf, shareKey)
		okB64 := base64.RawURLEncoding.EncodeToString(okBuf)

		// ha = AES-ECB(masterKey, folderHandle + folderHandle)
		authRaw := []byte(folderHandle + folderHandle)
		if len(authRaw) < 16 {
			authRaw = append(authRaw, make([]byte, 16-len(authRaw))...)
		} else if len(authRaw) > 16 {
			authRaw = authRaw[:16]
		}
		haBuf := make([]byte, 16)
		encryptECB(masterCipher, haBuf, authRaw)
		haB64 := base64.RawURLEncoding.EncodeToString(haBuf)

		// Map child nodes by parent to find all descendants
		byParent := make(map[string][]apiNode)
		for _, node := range fResp[0].F {
			byParent[node.Parent] = append(byParent[node.Parent], node)
		}

		var descendants []apiNode
		var collect func(h string)
		collect = func(h string) {
			for _, ch := range byParent[h] {
				descendants = append(descendants, ch)
				if ch.Type == 1 { // folder
					collect(ch.Hash)
				}
			}
		}
		collect(folderHandle)

		sources := []string{folderHandle}
		for _, d := range descendants {
			sources = append(sources, d.Hash)
		}

		shares := []string{folderHandle}
		crKeys := []any{}

		shareCipher, sErr := aes.NewCipher(shareKey)
		if sErr != nil {
			return "", "", "", fmt.Errorf("failed to init share cipher: %w", sErr)
		}

		// Re-encrypt keys of descendant nodes with shareKey for public access
		for idx, d := range descendants {
			sourceIdx := idx + 1
			if d.Key == "" {
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

	// Step 2: Now that the folder is an exported share, obtain the public link handle with "l" command
	lReq := []lPayload{{Cmd: "l", N: folderHandle}}
	lResBytes, lErr := callMegaAPI(sid, lReq)
	if lErr != nil {
		return "", "", "", fmt.Errorf("failed to export link handle: %w", lErr)
	}

	var lRes []string
	if err := json.Unmarshal(lResBytes, &lRes); err != nil || len(lRes) == 0 || lRes[0] == "" {
		return "", "", "", fmt.Errorf("could not parse public link id from response: %s", string(lResBytes))
	}

	linkHandle := lRes[0]
	keyB64 := base64.RawURLEncoding.EncodeToString(finalShareKey)
	publicURL := fmt.Sprintf("https://mega.nz/folder/%s#%s", linkHandle, keyB64)

	return linkHandle, keyB64, publicURL, nil
}
