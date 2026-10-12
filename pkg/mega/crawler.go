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

type AccountIndex struct {
	Email    string    `json:"email"`
	Password string    `json:"password"`
	Tree     *FileNode `json:"tree"`
}

// CrawlAllAccounts generates a unified file tree for all verified accounts
func CrawlAllAccounts(accounts []*storage.Account, megatoolsPath string) ([]*AccountIndex, error) {
	var indices []*AccountIndex

	for _, acc := range accounts {
		if acc.Status != "verified" {
			continue
		}

		accNode := &FileNode{Name: "Root", Path: "/Root", IsDir: true, Children: []*FileNode{}}

		// Run: megatools ls -l -R -u <email> -p <password>
		cmd := exec.Command(megatoolsPath, "ls", "-l", "-R", "-u", acc.Email, "-p", acc.MegaPassword)
		out, err := cmd.Output()
		if err == nil && len(out) > 0 {
			lines := strings.Split(string(out), "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				parts := strings.Fields(line)
				if len(parts) >= 1 {
					path := parts[len(parts)-1]
					if strings.HasPrefix(path, "/") {
						addToTree(accNode, path)
					}
				}
			}
		}

		indices = append(indices, &AccountIndex{
			Email:    acc.Email,
			Password: acc.MegaPassword,
			Tree:     accNode,
		})
	}

	return indices, nil
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
