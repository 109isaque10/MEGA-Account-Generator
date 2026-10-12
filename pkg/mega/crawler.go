package mega

import (
	"fmt"
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

// CrawlAllAccounts generates a unified file tree for all verified accounts with progress callbacks
func CrawlAllAccounts(accounts []*storage.Account, megatoolsPath string, onProgress func(completed, total int, currentEmail string)) ([]*AccountIndex, error) {
	var indices []*AccountIndex

	// Count verified accounts for accurate total
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

		accNode := &FileNode{Name: "Root", Path: "/Root", IsDir: true, Children: []*FileNode{}}

		// Run: megatools ls -l -R -u <email> -p <password>
		cmd := exec.Command(megatoolsPath, "ls", "-l", "-R", "-u", acc.Email, "-p", acc.MegaPassword)
		out, err := cmd.Output()
		if err == nil && len(out) > 0 {
			lines := strings.Split(string(out), "\n")
			for _, line := range lines {
				isDir, size, path, ok := parseMegatoolsLine(line)
				if ok && path != "" {
					addToTree(accNode, path, size, isDir)
				}
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

func parseMegatoolsLine(line string) (isDir bool, size int64, path string, ok bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return false, 0, "", false
	}
	parts := strings.Fields(line)
	if len(parts) >= 5 && (parts[0] == "d" || parts[0] == "-") {
		isDir = (parts[0] == "d")
		var s int64
		_, _ = fmt.Sscanf(parts[1], "%d", &s)
		size = s
		// The path is after parts[3] (time field). Everything after is the path (supports spaces).
		idx := strings.Index(line, parts[3])
		if idx != -1 {
			path = strings.TrimSpace(line[idx+len(parts[3]):])
		} else {
			path = strings.Join(parts[4:], " ")
		}
		return isDir, size, path, true
	} else if strings.HasPrefix(line, "/") {
		// Plain megatools ls output without -l
		isDir = !strings.Contains(line[strings.LastIndex(line, "/"):], ".")
		return isDir, 0, line, true
	}
	return false, 0, "", false
}

func addToTree(root *FileNode, fullPath string, size int64, isDir bool) {
	// Strip leading /Root or /
	clean := strings.TrimPrefix(fullPath, "/Root")
	clean = strings.Trim(clean, "/")
	if clean == "" {
		return
	}
	parts := strings.Split(clean, "/")
	curr := root
	for i, part := range parts {
		isLeaf := (i == len(parts)-1)
		found := false
		for _, child := range curr.Children {
			if child.Name == part {
				curr = child
				found = true
				if isLeaf {
					curr.IsDir = isDir
					curr.Size = size
				}
				break
			}
		}
		if !found {
			nodeIsDir := true
			var nodeSize int64
			if isLeaf {
				nodeIsDir = isDir
				nodeSize = size
			}
			newNode := &FileNode{
				Name:     part,
				Path:     fullPath,
				IsDir:    nodeIsDir,
				Size:     nodeSize,
				Children: []*FileNode{},
			}
			curr.Children = append(curr.Children, newNode)
			curr = newNode
		}
	}
}
