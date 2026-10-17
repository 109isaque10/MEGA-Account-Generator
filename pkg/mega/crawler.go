package mega

import (
	"fmt"
	"log"
	"strings"

	"github.com/109isaque10/mega-account-generator/pkg/storage"
	gomega "github.com/t3rm1n4l/go-mega"
)

type FileNode struct {
	Name     string      `json:"name"`
	Path     string      `json:"path"`
	Size     int64       `json:"size"`
	IsDir    bool        `json:"is_dir"`
	Hash     string      `json:"hash,omitempty"`
	Duration float64     `json:"duration_seconds,omitempty"`
	Children []*FileNode `json:"children,omitempty"`
}

type AccountIndex struct {
	Email              string    `json:"email"`
	Password           string    `json:"password"`
	PublicFolderHandle string    `json:"public_folder_handle,omitempty"`
	PublicFolderKey    string    `json:"public_folder_key,omitempty"`
	PublicFolderURL    string    `json:"public_folder_url,omitempty"`
	Tree               *FileNode `json:"tree"`
}

// CrawlAllAccounts generates a unified file tree for all verified accounts using go-mega
func CrawlAllAccounts(accounts []*storage.Account, megatoolsPath string, onProgress func(completed, total int, currentEmail string)) ([]*AccountIndex, error) {
	var indices []*AccountIndex

	var verified []*storage.Account
	for _, acc := range accounts {
		if acc.Status == "verified" {
			verified = append(verified, acc)
		}
	}

	total := len(verified)
	for i, acc := range verified {
		if onProgress != nil {
			onProgress(i, total, acc.Email)
		}

		accNode := &FileNode{Name: "/", Path: "/", IsDir: true, Children: []*FileNode{}}

		m := gomega.New()
		pubHandle := acc.PublicFolderHandle
		pubKey := acc.PublicFolderKey
		pubURL := acc.PublicFolderURL

		err := m.Login(acc.Email, acc.MegaPassword)
		if err != nil {
			log.Printf("[CRAWLER] go-mega login failed for %s: %v", acc.Email, err)
		} else {
			rootNode := m.FS.GetRoot()
			if rootNode != nil {
				crawlGoMegaNode(m, rootNode, "/Root", accNode)

				// Auto-export public link if missing
				if pubHandle == "" || pubKey == "" {
					if h, k, u, expErr := exportFolderForRoot(m, rootNode); expErr == nil && h != "" && k != "" {
						pubHandle = h
						pubKey = k
						pubURL = u
						acc.PublicFolderHandle = pubHandle
						acc.PublicFolderKey = pubKey
						acc.PublicFolderURL = pubURL
					}
				}
			}
		}

		indices = append(indices, &AccountIndex{
			Email:              acc.Email,
			Password:           acc.MegaPassword,
			PublicFolderHandle: pubHandle,
			PublicFolderKey:    pubKey,
			PublicFolderURL:    pubURL,
			Tree:               accNode,
		})

		if onProgress != nil {
			onProgress(i+1, total, acc.Email)
		}
	}

	return indices, nil
}

// ExportAccountPublicFolder exports a public link for the account's folder.
// Since MEGA prohibits exporting the Root drive node directly (API_EACCESS -11),
// this function targets an existing user folder (such as 'Public' or the top-level folder)
// or automatically creates a 'Public' directory and moves root files into it.
func ExportAccountPublicFolder(email, password string) (handle, key, publicURL string, err error) {
	m := gomega.New()
	if err := m.Login(email, password); err != nil {
		return "", "", "", fmt.Errorf("login failed for %s: %w", email, err)
	}

	rootNode := m.FS.GetRoot()
	if rootNode == nil {
		return "", "", "", fmt.Errorf("root node not found for %s", email)
	}

	return exportFolderForRoot(m, rootNode)
}

func exportFolderForRoot(m *gomega.Mega, rootNode *gomega.Node) (string, string, string, error) {
	children, err := m.FS.GetChildren(rootNode)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to list root children: %w", err)
	}

	var targetFolder *gomega.Node
	var folderNodes []*gomega.Node

	for _, ch := range children {
		if ch.GetType() == gomega.FOLDER {
			folderNodes = append(folderNodes, ch)
			nameLower := strings.ToLower(ch.GetName())
			if nameLower == "public" || nameLower == "shared" {
				targetFolder = ch
				break
			}
		}
	}

	// If no explicit "public" folder found, select from available folders
	if targetFolder == nil {
		if len(folderNodes) == 1 {
			targetFolder = folderNodes[0]
		} else if len(folderNodes) > 1 {
			targetFolder = folderNodes[0]
		}
	}

	// If no folder exists at all, create "Public" folder under root
	if targetFolder == nil {
		newFolder, cErr := m.CreateDir("Public", rootNode)
		if cErr != nil {
			return "", "", "", fmt.Errorf("failed to create Public directory: %w", cErr)
		}
		targetFolder = newFolder

		// Move any loose files in root into the new Public directory
		for _, ch := range children {
			if ch.GetType() == gomega.FILE {
				_ = m.Move(ch, targetFolder)
			}
		}
	}

	handle, key, publicURL, err := ExportFolderViaS2(m, targetFolder)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to export folder via s2: %w", err)
	}

	return handle, key, publicURL, nil
}

func parseLinkHandleAndKey(link string) (string, string) {
	link = strings.TrimSpace(link)
	if idx := strings.Index(link, "/folder/"); idx != -1 {
		rest := link[idx+len("/folder/"):]
		parts := strings.SplitN(rest, "#", 2)
		if len(parts) == 2 {
			return parts[0], parts[1]
		}
	}
	if idx := strings.Index(link, "/#F!"); idx != -1 {
		rest := link[idx+len("/#F!"):]
		parts := strings.SplitN(rest, "!", 2)
		if len(parts) == 2 {
			return parts[0], parts[1]
		}
	}
	if idx := strings.Index(link, "/#!"); idx != -1 {
		rest := link[idx+len("/#!"):]
		parts := strings.SplitN(rest, "!", 2)
		if len(parts) == 2 {
			return parts[0], parts[1]
		}
	}
	return "", ""
}

func crawlGoMegaNode(m *gomega.Mega, currNode *gomega.Node, currentPath string, parentFileNode *FileNode) {
	children, err := m.FS.GetChildren(currNode)
	if err != nil {
		return
	}

	for _, ch := range children {
		name := ch.GetName()
		nodeType := ch.GetType() // 0 = file, 1 = folder
		isDir := (nodeType == gomega.FOLDER)
		size := ch.GetSize()
		childPath := currentPath + "/" + name

		newNode := &FileNode{
			Name:     name,
			Path:     childPath,
			IsDir:    isDir,
			Size:     size,
			Hash:     ch.GetHash(),
			Children: []*FileNode{},
		}

		parentFileNode.Children = append(parentFileNode.Children, newNode)

		if isDir {
			crawlGoMegaNode(m, ch, childPath, newNode)
		}
	}
}
