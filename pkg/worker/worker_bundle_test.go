package worker_test

import (
	"strings"
	"testing"

	"github.com/109isaque10/mega-account-generator/pkg/worker"
)

func TestGenerateCloudflareWorkerBundle(t *testing.T) {
	files := worker.GenerateCloudflareWorker(nil)
	if len(files) != 3 {
		t.Fatalf("expected 3 files, got %d", len(files))
	}
	byName := map[string]string{}
	for _, f := range files {
		byName[f.Name] = f.Content
	}
	js, ok := byName["worker.js"]
	if !ok {
		t.Fatal("missing worker.js")
	}
	if strings.Contains(js, "const UNION_TREE =") {
		t.Error("worker.js still embeds the union tree inline")
	}
	if !strings.Contains(js, "getUnionTree") {
		t.Error("worker.js should load the tree via getUnionTree")
	}
	if !strings.Contains(js, "handleHLSPlaylist") {
		t.Error("worker.js missing handleHLSPlaylist route")
	}
	if !strings.Contains(js, "handleHLSSegment") {
		t.Error("worker.js missing handleHLSSegment route")
	}
	if !strings.Contains(js, "caches.default") {
		t.Error("worker.js missing Cloudflare edge caches.default integration")
	}
	if strings.Contains(js, "megaLogin") {
		t.Error("worker.js should not contain private megaLogin credentials code")
	}
	cfg, ok := byName["wrangler.jsonc"]
	if !ok {
		t.Fatal("missing wrangler.jsonc")
	}
	if !strings.Contains(cfg, `"binding": "ASSETS"`) {
		t.Error("wrangler.jsonc missing ASSETS binding")
	}
	tree, ok := byName["public/union-tree.json"]
	if !ok {
		t.Fatal("missing public/union-tree.json")
	}
	if !strings.HasPrefix(strings.TrimSpace(tree), "[") {
		t.Error("union-tree.json should be a JSON array")
	}
	t.Logf("sizes: worker.js=%dB wrangler.jsonc=%dB union-tree.json=%dB",
		len(js), len(cfg), len(byName["public/union-tree.json"]))
}
