package mega

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

var (
	urlRegex = regexp.MustCompile(`(?i)\bhttps?://[^\s<>"'\(\)\[\]]+`)
)

type Client struct {
	megatoolsPath string
}

func NewClient(megatoolsPath string) *Client {
	if megatoolsPath == "" {
		megatoolsPath = "megatools"
	}
	return &Client{megatoolsPath: megatoolsPath}
}

// GenerateRandomPassword generates a strong password of requested length
func GenerateRandomPassword(length int) string {
	if length < 10 {
		length = 14
	}
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*"
	res := make([]byte, length)
	for i := 0; i < length; i++ {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		if err != nil {
			res[i] = chars[i%len(chars)]
		} else {
			res[i] = chars[num.Int64()]
		}
	}
	return string(res)
}

func GenerateRandomName() string {
	firsts := []string{"Alex", "Jordan", "Taylor", "Morgan", "Sam", "Chris", "Pat", "Robin", "Casey", "Riley"}
	lasts := []string{"Smith", "Johnson", "Williams", "Brown", "Jones", "Garcia", "Miller", "Davis", "Rodriguez", "Martinez"}
	fIdx, _ := rand.Int(rand.Reader, big.NewInt(int64(len(firsts))))
	lIdx, _ := rand.Int(rand.Reader, big.NewInt(int64(len(lasts))))
	return fmt.Sprintf("%s %s", firsts[fIdx.Int64()], lasts[lIdx.Int64()])
}

// Register initiates the registration process with megatools.
// Returns the scripted verification command containing @LINK@ placeholder.
func (c *Client) Register(ctx context.Context, email, name, password string) (string, error) {
	cmd := exec.CommandContext(ctx, c.megatoolsPath, "reg", "--scripted", "--register",
		"--email", email,
		"--name", name,
		"--password", password,
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("megatools register failed: %v (stderr: %s)", err, stderr.String())
	}

	output := strings.TrimSpace(stdout.String())
	if output == "" {
		return "", fmt.Errorf("empty output from megatools register (stderr: %s)", stderr.String())
	}

	return output, nil
}

// Verify executes the verification command with the extracted link
func (c *Client) Verify(ctx context.Context, verifyCommandTemplate, confirmationURL string) error {
	finalCmdStr := strings.Replace(verifyCommandTemplate, "@LINK@", confirmationURL, 1)

	// In megatools, if verifyCommandTemplate is:
	// megatools reg --scripted --verify <state> @LINK@
	// we can execute via sh -c or parse arguments
	cmd := exec.CommandContext(ctx, "sh", "-c", finalCmdStr)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	combined := stdout.String() + " " + stderr.String()
	if err != nil && !strings.Contains(combined, "registered successfully!") {
		return fmt.Errorf("verification failed: %v (output: %s)", err, combined)
	}

	if strings.Contains(strings.ToLower(combined), "registered successfully!") || err == nil {
		return nil
	}

	return fmt.Errorf("verification output did not indicate success: %s", combined)
}

// LoginCheck runs megatools ls to confirm credentials and keep the account alive
func (c *Client) LoginCheck(ctx context.Context, email, password string) error {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, c.megatoolsPath, "ls", "-u", email, "-p", password)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	out := stdout.String()
	if strings.Contains(out, "/Root") || strings.Contains(out, "/Trash") {
		return nil
	}

	if err != nil {
		return fmt.Errorf("login failed: %v (stderr: %s)", err, stderr.String())
	}
	return nil
}

// ExtractConfirmationURL finds links from email plain body or html
func ExtractConfirmationURL(text string) (string, error) {
	urls := urlRegex.FindAllString(text, -1)
	for _, u := range urls {
		clean := strings.TrimRight(u, ".,;:\"'>)]}")
		if strings.Contains(clean, "mega.nz") || strings.Contains(clean, "mega.co.nz") {
			return clean, nil
		}
	}
	if len(urls) > 0 {
		return urls[0], nil
	}
	return "", fmt.Errorf("no URL found in email body")
}
