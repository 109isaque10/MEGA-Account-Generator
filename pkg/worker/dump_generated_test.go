package worker_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/109isaque10/mega-account-generator/pkg/storage"
	"github.com/109isaque10/mega-account-generator/pkg/worker"
)

// TestDumpGeneratedWorkerJS regenerates the worker bundle to /tmp for offline
// syntax/crypto checks. When MEGA_TEST_EMAIL is set (and accounts(1).json is
// present), that real account is injected so the live login path can be
// exercised against the MEGA API.
func TestDumpGeneratedWorkerJS(t *testing.T) {
	var accounts []*storage.Account
	if email := os.Getenv("MEGA_TEST_EMAIL"); email != "" {
		raw, err := os.ReadFile("../../accounts(1).json")
		if err != nil {
			t.Skipf("accounts file not available: %v", err)
		}
		var list []*storage.Account
		if err := json.Unmarshal(raw, &list); err != nil {
			t.Fatalf("bad accounts json: %v", err)
		}
		for _, acc := range list {
			if strings.EqualFold(acc.Email, email) {
				acc.Status = "verified"
				accounts = append(accounts, acc)
				break
			}
		}
		if len(accounts) == 0 {
			t.Fatalf("account %q not found in accounts(1).json", email)
		}
	}

	files := worker.GenerateCloudflareWorker(accounts)
	for _, f := range files {
		if f.Name == "worker.js" {
			if err := os.WriteFile("/tmp/generated-worker.js", []byte(f.Content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
}
