package mega

import (
	"log"

	"github.com/109isaque10/mega-account-generator/pkg/storage"
	gomega "github.com/t3rm1n4l/go-mega"
)

type FileNode struct {
	Name     string      `json:"name"`
	Path     string      `json:"path"`
	Size     int64       `json:"size"`
	IsDir    bool        `json:"is_dir"`
	Hash     string      `json:"hash,omitempty"`
	Children []*FileNode `json:"children,omitempty"`
}

type AccountIndex struct {
	Email    string    `json:"email"`
	Password string    `json:"password"`
	Tree     *FileNode `json:"tree"`
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
		err := m.Login(acc.Email, acc.MegaPassword)
		if err != nil {
			log.Printf("[CRAWLER] go-mega login failed for %s: %v", acc.Email, err)
		} else {
			rootNode := m.FS.GetRoot()
			if rootNode != nil {
				crawlGoMegaNode(m, rootNode, "/Root", accNode)
			}
		}

		indices = append(indices, &AccountIndex{
			Email:    acc.Email,
			Password: acc.MegaPassword,
			Tree:     accNode,
		})

		if onProgress != nil {
			onProgress(i+1, total, acc.Email)
		}
	}

	return indices, nil
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
