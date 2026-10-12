package mega

import (
	"os/exec"
	"strings"

	"github.com/109isaque10/mega-account-generator/pkg/storage"
)

type FileNode struct {
	Name     string      `json:"name"`
	Path     string      `json:"path"`
	Size     int64       `json:"size"`
	IsDir    bool        `json:"is_dir"`
	Children []*FileNode `json:"children,omitempty"`
}

// CrawlAllAccounts generates a unified file tree for all verified accounts
func CrawlAllAccounts(accounts []*storage.Account, megatoolsPath string) (*FileNode, error) {
	root := &FileNode{Name: "root", IsDir: true, Children: []*FileNode{}}

	for _, acc := range accounts {
		if acc.Status != "verified" {
			continue
		}

		accNode := &FileNode{Name: acc.Email, IsDir: true, Children: []*FileNode{}}
		
		// Run: megatools ls -R /
		cmd := exec.Command(megatoolsPath, "--username", acc.Email, "--password", acc.MegaPassword, "ls", "-R", "/")
		out, err := cmd.Output()
		if err != nil {
			continue // Skip accounts that fail to list
		}

		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			if strings.TrimSpace(line) == "" { continue }
			// Simplified parsing: expecting "path size"
			parts := strings.Fields(line)
			if len(parts) < 2 { continue }
			path := parts[0]
			// Recursive logic to build tree...
			addToTree(accNode, path)
		}
		root.Children = append(root.Children, accNode)
	}

	return root, nil
}

func addToTree(root *FileNode, path string) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	curr := root
	for _, part := range parts {
		found := false
		for _, child := range curr.Children {
			if child.Name == part {
				curr = child
				found = true
				break
			}
		}
		if !found {
			newNode := &FileNode{Name: part, IsDir: true, Children: []*FileNode{}}
			curr.Children = append(curr.Children, newNode)
			curr = newNode
		}
	}
}
