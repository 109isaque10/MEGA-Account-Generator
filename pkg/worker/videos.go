package worker

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/109isaque10/mega-account-generator/pkg/mega"
	"github.com/109isaque10/mega-account-generator/pkg/storage"
)

type VideoExportItem struct {
	Name            string   `json:"name"`
	Path            string   `json:"path"`
	Size            int64    `json:"size"`
	DownloadURL     string   `json:"download_url"`
	StreamURL       string   `json:"stream_url,omitempty"`
	HLSURL          string   `json:"hls_url,omitempty"`
	Account         string   `json:"account,omitempty"`
	DurationSeconds *float64 `json:"duration_seconds,omitempty"`
}

func isVideoFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".mp4", ".mkv", ".webm", ".avi", ".mov", ".m4v", ".flv", ".wmv", ".ts":
		return true
	default:
		return false
	}
}

// ExtractVideoCatalog scans the current file index and extracts all video files with their paths,
// sizes, download links, and duration (in seconds) if available.
func ExtractVideoCatalog(accounts []*storage.Account) ([]VideoExportItem, error) {
	pubFolderMap := make(map[string]PublicFolderConfig)
	accountMap := make(map[string]string)
	idx := 1

	for _, acc := range accounts {
		if acc.Status == "verified" {
			remoteID := fmt.Sprintf("mega_%d", idx)
			accountMap[acc.Email] = remoteID
			pubFolderMap[acc.Email] = PublicFolderConfig{
				ID:     remoteID,
				Email:  acc.Email,
				Handle: acc.PublicFolderHandle,
				Key:    acc.PublicFolderKey,
				URL:    acc.PublicFolderURL,
			}
			idx++
		}
	}

	candidatePaths := []string{
		"data/index.json",
		"index.json",
	}
	if envDir := os.Getenv("DATA_DIR"); envDir != "" {
		candidatePaths = append([]string{filepath.Join(envDir, "index.json")}, candidatePaths...)
	}

	results := make([]VideoExportItem, 0)

	for _, p := range candidatePaths {
		data, err := os.ReadFile(p)
		if err != nil || len(data) < 5 {
			continue
		}

		var accountIndices []*mega.AccountIndex
		if err := json.Unmarshal(data, &accountIndices); err == nil && len(accountIndices) > 0 && accountIndices[0].Email != "" {
			for _, ai := range accountIndices {
				remoteID := accountMap[ai.Email]
				if remoteID == "" {
					remoteID = ai.Email
				}
				pub := pubFolderMap[ai.Email]
				if pub.Handle == "" && ai.PublicFolderHandle != "" {
					pub.Handle = ai.PublicFolderHandle
					pub.Key = ai.PublicFolderKey
					pub.URL = ai.PublicFolderURL
				}

				if ai.Tree != nil {
					collectVideosFromNode(ai.Tree, remoteID, pub.Handle, pub.Key, &results)
				}
			}
			return results, nil
		}

		var unionItems []*UnionItem
		if err := json.Unmarshal(data, &unionItems); err == nil && len(unionItems) > 0 && unionItems[0].Name != "" {
			collectVideosFromUnion(unionItems, &results)
			return results, nil
		}
	}

	return results, nil
}

func collectVideosFromNode(node *mega.FileNode, accountTag, pubHandle, pubKey string, results *[]VideoExportItem) {
	if node == nil {
		return
	}
	if !node.IsDir && isVideoFile(node.Name) {
		downloadURL := ""
		streamURL := ""
		hlsURL := ""
		if pubHandle != "" && pubKey != "" && node.Hash != "" {
			downloadURL = "/p/" + pubHandle + "/" + pubKey + "/download/" + node.Hash
			streamURL = "/stream/" + pubHandle + "/" + pubKey + "/" + node.Hash
			hlsURL = "/hls/" + pubHandle + "/" + pubKey + "/" + node.Hash + "/playlist.m3u8"
		}

		item := VideoExportItem{
			Name:        node.Name,
			Path:        node.Path,
			Size:        node.Size,
			DownloadURL: downloadURL,
			StreamURL:   streamURL,
			HLSURL:      hlsURL,
			Account:     accountTag,
		}

		if node.Duration > 0 {
			dur := node.Duration
			item.DurationSeconds = &dur
		}

		*results = append(*results, item)
	}

	for _, ch := range node.Children {
		collectVideosFromNode(ch, accountTag, pubHandle, pubKey, results)
	}
}

func collectVideosFromUnion(items []*UnionItem, results *[]VideoExportItem) {
	for _, itm := range items {
		if itm == nil {
			continue
		}
		if !itm.IsDir && isVideoFile(itm.Name) {
			item := VideoExportItem{
				Name:        itm.Name,
				Path:        itm.Path,
				Size:        itm.Size,
				DownloadURL: itm.DownloadURL,
				StreamURL:   itm.StreamURL,
				HLSURL:      itm.HLSURL,
				Account:     itm.Account,
			}
			if itm.DurationSeconds > 0 {
				dur := itm.DurationSeconds
				item.DurationSeconds = &dur
			}
			*results = append(*results, item)
		}
		if len(itm.Children) > 0 {
			collectVideosFromUnion(itm.Children, results)
		}
	}
}
