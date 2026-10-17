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

type PublicFolderConfig struct {
	ID     string `json:"id"`
	Email  string `json:"email"`
	Handle string `json:"handle"`
	Key    string `json:"key"`
	URL    string `json:"url"`
}

type UnionItem struct {
	Name            string       `json:"name"`
	Path            string       `json:"path"`
	IsDir           bool         `json:"is_dir"`
	Size            int64        `json:"size"`
	Account         string       `json:"account"`
	Hash            string       `json:"hash,omitempty"`
	DurationSeconds float64      `json:"duration_seconds,omitempty"`
	DownloadURL     string       `json:"download_url,omitempty"`
	StreamURL       string       `json:"stream_url,omitempty"`
	HLSURL          string       `json:"hls_url,omitempty"`
	PlayURL         string       `json:"play_url,omitempty"`
	Children        []*UnionItem `json:"children,omitempty"`
}

// GenerateStandaloneHTML generates a self-contained HTML page
func GenerateStandaloneHTML(accounts []*storage.Account) string {
	var rows strings.Builder
	for idx, acc := range accounts {
		if acc.Status != "verified" {
			continue
		}
		pubLink := acc.PublicFolderURL
		if pubLink == "" {
			pubLink = "-"
		}
		rows.WriteString(fmt.Sprintf(`
      <tr>
        <td>%d</td>
        <td><code>%s</code></td>
        <td><code>%s</code></td>
        <td><code>mega_%d:</code></td>
        <td><code>%s</code></td>
        <td>
          <button onclick="navigator.clipboard.writeText('%s:%s'); alert('Copied');">Copy Credentials</button>
        </td>
      </tr>`, idx+1, acc.Email, acc.MegaPassword, idx+1, pubLink, acc.Email, acc.MegaPassword))
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>MEGA Union Directory</title>
  <style>
    body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; background: #0f172a; color: #f8fafc; padding: 2rem; max-width: 900px; margin: 0 auto; }
    h1 { color: #38bdf8; font-size: 1.5rem; margin-bottom: 0.5rem; }
    p { color: #94a3b8; font-size: 0.9rem; margin-bottom: 1.5rem; }
    table { width: 100%%; border-collapse: collapse; background: #1e293b; border-radius: 8px; overflow: hidden; }
    th, td { padding: 0.75rem 1rem; text-align: left; border-bottom: 1px solid #334155; font-size: 0.85rem; }
    th { background: #334155; color: #cbd5e1; font-weight: 600; }
    code { font-family: monospace; background: #0f172a; padding: 0.2rem 0.4rem; border-radius: 4px; color: #38bdf8; }
    button { background: #0284c7; color: white; border: none; padding: 0.4rem 0.8rem; border-radius: 4px; cursor: pointer; font-weight: 600; }
  </style>
</head>
<body>
  <h1>MEGA Accounts & Union Directory</h1>
  <p>Self-contained directory of verified accounts and public folder links.</p>
  <table>
    <thead><tr><th>#</th><th>Email</th><th>Password</th><th>Remote</th><th>Public Folder</th><th>Action</th></tr></thead>
    <tbody>%s</tbody>
  </table>
</body>
</html>`, rows.String())
}

func convertFileNode(node *mega.FileNode, accountTag, pubHandle, pubKey string) *UnionItem {
	item := &UnionItem{
		Name:     node.Name,
		Path:     node.Path,
		IsDir:    node.IsDir,
		Size:     node.Size,
		Account:         accountTag,
		Hash:            node.Hash,
		DurationSeconds: node.Duration,
		Children:        make([]*UnionItem, 0, len(node.Children)),
	}
	if !node.IsDir && node.Hash != "" {
		if pubHandle != "" && pubKey != "" {
			item.DownloadURL = "/p/" + pubHandle + "/" + pubKey + "/download/" + node.Hash
			item.StreamURL = "/stream/" + pubHandle + "/" + pubKey + "/" + node.Hash
			item.HLSURL = "/hls/" + pubHandle + "/" + pubKey + "/" + node.Hash + "/playlist.m3u8"
			item.PlayURL = "/play/" + pubHandle + "/" + pubKey + "/" + node.Hash
		} else if accountTag != "" {
			item.DownloadURL = "/p/pending/pending/download/" + node.Hash
		}
	}
	for _, ch := range node.Children {
		item.Children = append(item.Children, convertFileNode(ch, accountTag, pubHandle, pubKey))
	}
	return item
}

// mergeNodeIntoUnion merges a file or folder into the union tree (rclone union style).
func mergeNodeIntoUnion(targetList []*UnionItem, item *UnionItem) []*UnionItem {
	if item.IsDir {
		for _, existing := range targetList {
			if existing.IsDir && existing.Name == item.Name {
				for _, ch := range item.Children {
					existing.Children = mergeNodeIntoUnion(existing.Children, ch)
				}
				return targetList
			}
		}
		return append(targetList, item)
	}

	return append(targetList, item)
}

// WorkerFile represents a single file of the generated Cloudflare Worker bundle.
type WorkerFile struct {
	Name    string
	Content string
}

// GenerateCloudflareWorker generates the Cloudflare Worker as a multi-file bundle:
// worker.js (public link HLS/download routing), wrangler.jsonc, and public/union-tree.json.
func GenerateCloudflareWorker(accounts []*storage.Account) []WorkerFile {
	var publicFolders []PublicFolderConfig
	pubFolderMap := make(map[string]PublicFolderConfig)
	accountMap := make(map[string]string)
	idx := 1

	for _, acc := range accounts {
		if acc.Status == "verified" {
			remoteID := fmt.Sprintf("mega_%d", idx)
			accountMap[acc.Email] = remoteID

			cfg := PublicFolderConfig{
				ID:     remoteID,
				Email:  acc.Email,
				Handle: acc.PublicFolderHandle,
				Key:    acc.PublicFolderKey,
				URL:    acc.PublicFolderURL,
			}
			publicFolders = append(publicFolders, cfg)
			pubFolderMap[acc.Email] = cfg
			idx++
		}
	}

	foldersJSON, _ := json.MarshalIndent(publicFolders, "", "  ")

	candidatePaths := []string{
		"data/index.json",
		"index.json",
	}
	if envDir := os.Getenv("DATA_DIR"); envDir != "" {
		candidatePaths = append([]string{filepath.Join(envDir, "index.json")}, candidatePaths...)
	}

	var unionTree []*UnionItem
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
				// Fallback to index-embedded public folder if not yet in accounts list
				if pub.Handle == "" && ai.PublicFolderHandle != "" {
					pub.Handle = ai.PublicFolderHandle
					pub.Key = ai.PublicFolderKey
					pub.URL = ai.PublicFolderURL
				}

				if ai.Tree != nil {
					for _, ch := range ai.Tree.Children {
						unionItem := convertFileNode(ch, remoteID, pub.Handle, pub.Key)
						unionTree = mergeNodeIntoUnion(unionTree, unionItem)
					}
				}
			}
			break
		}

		var directItems []*UnionItem
		if err := json.Unmarshal(data, &directItems); err == nil && len(directItems) > 0 && directItems[0].Name != "" {
			unionTree = directItems
			break
		}
	}

	if unionTree == nil {
		unionTree = make([]*UnionItem, 0)
	}

	unionJSON, _ := json.Marshal(unionTree)

	return []WorkerFile{
		{Name: "worker.js", Content: buildWorkerJS(string(foldersJSON))},
		{Name: "wrangler.jsonc", Content: buildWranglerConfig()},
		{Name: "public/union-tree.json", Content: string(unionJSON) + "\n"},
	}
}

func buildWranglerConfig() string {
	return `{
  // MEGA.nz Cloudflare Worker - Anonymous Public Folder & HLS Video CDN
  // Deploy with: npx wrangler deploy
  "name": "mega-video-cdn",
  "main": "worker.js",
  "compatibility_date": "2026-10-12",
  "assets": {
    "directory": "./public",
    "binding": "ASSETS",
    "run_worker_first": true
  }
}
`
}

func buildWorkerJS(foldersJSON string) string {
	return `// MEGA.nz Cloudflare Worker - Anonymous Public Folder & HLS Video CDN
// Pure anonymous public link streaming with Cloudflare edge caching.
// Zero account logins, zero stored passwords, zero risk of account lockouts.

const SHARED_FOLDERS = ` + foldersJSON + `;
const publicTreeCache = {};

let unionTreeCache = null;
async function getUnionTree(env, origin) {
  if (unionTreeCache) return unionTreeCache;
  if (!env || !env.ASSETS) {
    throw new Error("ASSETS binding unavailable. Deploy with 'npx wrangler deploy'.");
  }
  const res = await env.ASSETS.fetch(new URL("/union-tree.json", origin));
  if (!res.ok) {
    throw new Error("Failed to load union-tree.json asset (HTTP " + res.status + ")");
  }
  unionTreeCache = await res.json();
  return unionTreeCache;
}

export default {
  async fetch(request, env, ctx) {
    const url = new URL(request.url);

    try {
      // 1. Root / Union file tree index
      if (url.pathname === "/" || url.pathname === "/union" || url.pathname.startsWith("/u/")) {
        const subPath = url.pathname.startsWith("/u/") ? decodeURIComponent(url.pathname.substring(3)) : "";
        const unionTree = await getUnionTree(env, url.origin);
        return renderUnionUI(unionTree, subPath, url.origin);
      }

      // 2. Union raw JSON index endpoint
      if (url.pathname === "/api/union.json") {
        const unionTree = await getUnionTree(env, url.origin);
        return new Response(JSON.stringify(unionTree, null, 2), {
          headers: { "Content-Type": "application/json", "Access-Control-Allow-Origin": "*" }
        });
      }

      // 3. Dynamic HLS Playlist: /hls/HANDLE/KEY/HASH/playlist.m3u8
      const hlsPlaylistMatch = url.pathname.match(/^\/hls\/([a-zA-Z0-9_-]+)\/([a-zA-Z0-9_-]+)\/([a-zA-Z0-9_-]+)\/playlist\.m3u8$/);
      if (hlsPlaylistMatch) {
        const [, folderHandle, keyB64, nodeHash] = hlsPlaylistMatch;
        return await handleHLSPlaylist(folderHandle, keyB64, nodeHash);
      }

      // 4. Cached HLS Segment: /hls/HANDLE/KEY/HASH/seg-(\d+)\.ts
      const hlsSegMatch = url.pathname.match(/^\/hls\/([a-zA-Z0-9_-]+)\/([a-zA-Z0-9_-]+)\/([a-zA-Z0-9_-]+)\/seg-(\d+)\.ts$/);
      if (hlsSegMatch) {
        const [, folderHandle, keyB64, nodeHash, segIdxStr] = hlsSegMatch;
        return await handleHLSSegment(request, folderHandle, keyB64, nodeHash, parseInt(segIdxStr, 10), ctx);
      }

      // 5. Embedded Web Video Player: /play/HANDLE/KEY/HASH
      const playMatch = url.pathname.match(/^\/play\/([a-zA-Z0-9_-]+)\/([a-zA-Z0-9_-]+)\/([a-zA-Z0-9_-]+)\/?$/);
      if (playMatch) {
        const [, folderHandle, keyB64, nodeHash] = playMatch;
        return await renderHLSPlayer(folderHandle, keyB64, nodeHash, url.origin);
      }

      // 6. Direct Range Video Stream: /stream/HANDLE/KEY/HASH
      const streamMatch = url.pathname.match(/^\/stream\/([a-zA-Z0-9_-]+)\/([a-zA-Z0-9_-]+)\/([a-zA-Z0-9_-]+)$/);
      if (streamMatch) {
        const [, folderHandle, keyB64, nodeHash] = streamMatch;
        return await handlePublicStream(request, folderHandle, keyB64, nodeHash);
      }

      // 7. Public folder directory view: /p/HANDLE/KEY
      const pubFolderMatch = url.pathname.match(/^\/p\/([a-zA-Z0-9_-]+)\/([a-zA-Z0-9_-]+)\/?$/);
      if (pubFolderMatch) {
        const folderHandle = pubFolderMatch[1];
        const keyB64 = pubFolderMatch[2];
        const forceRefresh = url.searchParams.get("refresh") === "1";
        const tree = await getOrFetchPublicTree(folderHandle, keyB64, forceRefresh);
        const sub = url.searchParams.get("sub") || tree.rootHash;
        return renderUI(tree, sub, url.origin, {
          basePath: "/p/" + folderHandle + "/" + keyB64,
          title: "Public MEGA Folder",
          subtitle: "Browsing a public mega.nz shared folder"
        });
      }

      // 8. Public folder direct download: /p/HANDLE/KEY/download/HASH
      const pubDownloadMatch = url.pathname.match(/^\/p\/([a-zA-Z0-9_-]+)\/([a-zA-Z0-9_-]+)\/download\/([a-zA-Z0-9_-]+)$/);
      if (pubDownloadMatch) {
        const [, folderHandle, keyB64, nodeHash] = pubDownloadMatch;
        return await handlePublicFolderDownload(request, folderHandle, keyB64, nodeHash);
      }

      // 9. Public single-file download: /pf/HANDLE/KEY/download
      const pubFileDownloadMatch = url.pathname.match(/^\/pf\/([a-zA-Z0-9_-]+)\/([a-zA-Z0-9_-]+)\/download$/);
      if (pubFileDownloadMatch) {
        const [, fileHandle, keyB64] = pubFileDownloadMatch;
        return await handlePublicFileDownload(request, fileHandle, keyB64);
      }

      // 10. Public single-file info: /pf/HANDLE/KEY
      const pubFileMatch = url.pathname.match(/^\/pf\/([a-zA-Z0-9_-]+)\/([a-zA-Z0-9_-]+)\/?$/);
      if (pubFileMatch) {
        const [, fileHandle, keyB64] = pubFileMatch;
        const meta = await fetchPublicFileMeta(fileHandle, keyB64);
        return renderFileUI(meta, "/pf/" + fileHandle + "/" + keyB64);
      }

      // 11. Public link opener: /open?link=...
      if (url.pathname === "/open") {
        const link = url.searchParams.get("link") || "";
        const parsed = parseMegaPublicLink(link);
        if (!parsed) {
          return new Response("Invalid MEGA public link.", { status: 400 });
        }
        const dest = parsed.type === "folder"
          ? "/p/" + parsed.handle + "/" + parsed.key
          : "/pf/" + parsed.handle + "/" + parsed.key;
        return Response.redirect(url.origin + dest, 302);
      }

      return new Response("Not Found", { status: 404 });
    } catch (err) {
      return new Response("Error: " + err.message + "\n" + err.stack, { status: 500 });
    }
  }
};

// --- HLS Streaming Handlers with Cloudflare Edge Caching ---

async function handleHLSPlaylist(folderHandle, keyB64, nodeHash) {
  const tree = await getOrFetchPublicTree(folderHandle, keyB64, false);
  const node = tree.nodes[nodeHash];
  if (!node || node.type !== 0) return new Response("File Not Found", { status: 404 });

  const fileSize = node.size || 0;
  const CHUNK_SIZE = 2 * 1024 * 1024; // 2 MB per segment
  const totalSegments = Math.max(1, Math.ceil(fileSize / CHUNK_SIZE));
  const targetDuration = 4;

  let m3u8 = "#EXTM3U\n";
  m3u8 += "#EXT-X-VERSION:3\n";
  m3u8 += "#EXT-X-TARGETDURATION:" + targetDuration + "\n";
  m3u8 += "#EXT-X-MEDIA-SEQUENCE:0\n";
  m3u8 += "#EXT-X-PLAYLIST-TYPE:VOD\n";

  for (let i = 0; i < totalSegments; i++) {
    m3u8 += "#EXTINF:" + targetDuration + ".0,\n";
    m3u8 += "seg-" + i + ".ts\n";
  }
  m3u8 += "#EXT-X-ENDLIST\n";

  return new Response(m3u8, {
    status: 200,
    headers: {
      "Content-Type": "application/vnd.apple.mpegurl",
      "Cache-Control": "public, max-age=3600",
      "Access-Control-Allow-Origin": "*"
    }
  });
}

async function handleHLSSegment(request, folderHandle, keyB64, nodeHash, segIndex, ctx) {
  const cache = caches.default;
  const cacheKey = new Request(request.url, { method: "GET" });
  let cached = await cache.match(cacheKey);
  if (cached) return cached;

  const tree = await getOrFetchPublicTree(folderHandle, keyB64, false);
  const node = tree.nodes[nodeHash];
  if (!node || node.type !== 0) return new Response("File Not Found", { status: 404 });

  const CHUNK_SIZE = 2 * 1024 * 1024; // 2 MB segments
  const start = segIndex * CHUNK_SIZE;
  if (start >= node.size) return new Response("Segment out of range", { status: 416 });
  const end = Math.min((segIndex + 1) * CHUNK_SIZE - 1, node.size - 1);

  const sn = Math.floor(Math.random() * 2147483647);
  const apiRes = await fetch("https://g.api.mega.co.nz/cs?id=" + sn + "&n=" + folderHandle, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify([{ a: "g", g: 1, n: nodeHash }])
  });
  const apiData = await apiRes.json();
  if (!apiData || !apiData[0] || !apiData[0].g) {
    return new Response("Failed to obtain download URL from MEGA", { status: 502 });
  }

  const chunkRes = await fetch(apiData[0].g, {
    headers: { "Range": "bytes=" + start + "-" + end }
  });
  if (!chunkRes.ok && chunkRes.status !== 206) {
    return new Response("Upstream MEGA chunk fetch error: " + chunkRes.status, { status: 502 });
  }

  const encrypted = new Uint8Array(await chunkRes.arrayBuffer());
  const cryptoKey = await crypto.subtle.importKey("raw", node.key, { name: "AES-CTR" }, false, ["decrypt"]);
  const iv = getCtrIv(node.iv, start);
  const decrypted = new Uint8Array(await crypto.subtle.decrypt({ name: "AES-CTR", counter: iv, length: 64 }, cryptoKey, encrypted));

  const response = new Response(decrypted, {
    status: 200,
    headers: {
      "Content-Type": "video/mp2t",
      "Content-Length": decrypted.byteLength.toString(),
      "Cache-Control": "public, max-age=604800, s-maxage=2592000",
      "Access-Control-Allow-Origin": "*",
      "Accept-Ranges": "bytes"
    }
  });

  if (ctx && ctx.waitUntil) {
    ctx.waitUntil(cache.put(cacheKey, response.clone()));
  }

  return response;
}

async function handlePublicStream(request, folderHandle, keyB64, nodeHash) {
  const tree = await getOrFetchPublicTree(folderHandle, keyB64, false);
  const node = tree.nodes[nodeHash];
  if (!node || node.type !== 0) return new Response("File Not Found", { status: 404 });
  const sn = Math.floor(Math.random() * 2147483647);
  const apiRes = await fetch("https://g.api.mega.co.nz/cs?id=" + sn + "&n=" + folderHandle, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify([{ a: "g", g: 1, n: nodeHash }])
  });
  const apiData = await apiRes.json();
  if (!apiData || !apiData[0] || !apiData[0].g) return new Response("Failed to obtain download URL", { status: 502 });
  return streamMegaFile(request, { size: node.size || 0, name: node.name, key: node.key, iv: node.iv }, apiData[0].g);
}

async function renderHLSPlayer(folderHandle, keyB64, nodeHash, origin) {
  const tree = await getOrFetchPublicTree(folderHandle, keyB64, false);
  const node = tree.nodes[nodeHash];
  const name = node ? node.name : "Video Stream";
  const playlistUrl = "/hls/" + folderHandle + "/" + keyB64 + "/" + nodeHash + "/playlist.m3u8";
  const downloadUrl = "/p/" + folderHandle + "/" + keyB64 + "/download/" + nodeHash;

  const html = '<!DOCTYPE html>' +
'<html lang="en">' +
'<head>' +
'<meta charset="UTF-8">' +
'<meta name="viewport" content="width=device-width, initial-scale=1.0">' +
'<title>' + escapeHtml(name) + ' - Video Stream</title>' +
'<script src="https://cdn.tailwindcss.com"></script>' +
'<script src="https://cdn.jsdelivr.net/npm/hls.js@latest"></script>' +
'<style>body{background:#07080e;color:#e2e8f0;font-family:sans-serif}.glass{background:rgba(255,255,255,0.03);border:1px solid rgba(255,255,255,0.08);backdrop-filter:blur(16px)}</style>' +
'</head>' +
'<body class="min-h-screen p-4 md:p-8 flex flex-col items-center justify-center">' +
  '<div class="w-full max-w-4xl glass rounded-3xl p-6 shadow-2xl flex flex-col gap-4">' +
    '<div class="flex items-center justify-between">' +
      '<div class="min-w-0 pr-4">' +
        '<h1 class="text-lg font-bold text-slate-100 truncate">' + escapeHtml(name) + '</h1>' +
        '<p class="text-xs text-slate-400 mt-0.5">Anonymous HLS CDN · Cloudflare Cached</p>' +
      '</div>' +
      '<div class="flex gap-2 shrink-0">' +
        '<a href="/" class="bg-white/5 hover:bg-white/10 text-slate-300 text-xs font-semibold px-3.5 py-2 rounded-xl transition-colors">Catalog</a>' +
        '<a href="' + downloadUrl + '" class="bg-indigo-600 hover:bg-indigo-500 text-white text-xs font-semibold px-3.5 py-2 rounded-xl transition-colors">Direct Download</a>' +
      '</div>' +
    '</div>' +
    '<div class="relative bg-black rounded-2xl overflow-hidden aspect-video shadow-inner">' +
      '<video id="video" controls class="w-full h-full object-contain" autoplay playsinline></video>' +
    '</div>' +
    '<div class="flex items-center justify-between text-xs text-slate-400">' +
      '<span>HLS Playlist: <code class="text-cyan-400 select-all">' + playlistUrl + '</code></span>' +
      '<button onclick="navigator.clipboard.writeText(location.origin + \'' + playlistUrl + '\');alert(\'HLS URL Copied!\')" class="text-indigo-400 hover:text-indigo-300 font-medium">Copy Stream Link</button>' +
    '</div>' +
  '</div>' +
  '<script>' +
    'const video = document.getElementById("video");' +
    'const src = "' + playlistUrl + '";' +
    'if (Hls.isSupported()) {' +
      'const hls = new Hls({ enableWorker: true, lowLatencyMode: true });' +
      'hls.loadSource(src);' +
      'hls.attachMedia(video);' +
    '} else if (video.canPlayType("application/vnd.apple.mpegurl")) {' +
      'video.src = src;' +
    '}' +
  '</script>' +
'</body></html>';

  return new Response(html, { headers: { "Content-Type": "text/html; charset=UTF-8" } });
}

// --- Union File Explorer UI ---
function renderUnionUI(tree, currentPath, origin) {
  const parts = currentPath.split("/").filter(Boolean);
  let currentItems = tree;
  let breadcrumbTrail = [{ name: "Union Root", path: "" }];
  let runningPath = "";

  for (const part of parts) {
    runningPath = runningPath ? runningPath + "/" + part : part;
    const found = currentItems.find(item => item.name === part && item.is_dir);
    if (found) {
      breadcrumbTrail.push({ name: found.name, path: runningPath });
      currentItems = found.children || [];
    } else {
      break;
    }
  }

  const breadcrumbsHtml = breadcrumbTrail.map((b, idx) => {
    const isLast = idx === breadcrumbTrail.length - 1;
    if (isLast) {
      return '<span class="text-indigo-300 font-semibold truncate max-w-[120px] md:max-w-xs">' + escapeHtml(b.name) + '</span>';
    }
    const target = b.path ? "/u/" + encodeURIComponent(b.path) : "/";
    return '<a href="' + target + '" class="hover:text-white transition-colors truncate max-w-[100px] md:max-w-[180px]">' + escapeHtml(b.name) + '</a><span class="text-gray-600 mx-2 shrink-0">/</span>';
  }).join("");

  let itemsHtml = "";
  if (!currentItems || currentItems.length === 0) {
    itemsHtml = '<div class="text-center py-20 flex flex-col items-center justify-center"><span class="text-gray-500 text-base font-medium">No files or folders found. Run "Sync File Index" in your dashboard to scan accounts.</span></div>';
  } else {
    itemsHtml = currentItems.map(item => {
      const isFolder = item.is_dir;
      const isVideo = /\.(mp4|mkv|webm|avi|mov|m4v)$/i.test(item.name);
      const icon = isFolder
        ? '<svg class="w-10 h-10 text-indigo-400 group-hover:scale-110 transition-transform duration-200" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z"></path></svg>'
        : '<svg class="w-10 h-10 text-cyan-400 group-hover:scale-110 transition-transform duration-200" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 21h10a2 2 0 002-2V9.414a1 1 0 00-.293-.707l-5.414-5.414A1 1 0 0012.586 3H7a2 2 0 00-2 2v14a2 2 0 002 2z"></path></svg>';

      const sub = currentPath ? currentPath + "/" + item.name : item.name;
      const targetUrl = isFolder
        ? "/u/" + encodeURIComponent(sub)
        : (item.play_url || item.download_url || "#");
      const accountBadge = item.account ? '<span class="text-[10px] bg-indigo-950 text-indigo-300 border border-indigo-700/50 px-2 py-0.5 rounded-full ml-2">' + escapeHtml(item.account) + '</span>' : '';

      let buttons = "";
      if (isFolder) {
        buttons = '<button class="bg-white/5 hover:bg-white/10 text-gray-300 hover:text-white px-3.5 py-1.5 rounded-lg text-xs font-semibold border border-white/5 transition-all">Open</button>';
      } else {
        if (isVideo && item.play_url) {
          buttons += '<a href="' + item.play_url + '" class="bg-cyan-600 hover:bg-cyan-500 text-white px-3 py-1.5 rounded-lg text-xs font-semibold shadow-lg shadow-cyan-600/20 mr-2 transition-all">Play HLS</a>';
        }
        if (item.download_url && item.download_url !== "#") {
          buttons += '<a href="' + item.download_url + '" class="bg-indigo-600 hover:bg-indigo-500 text-white px-3 py-1.5 rounded-lg text-xs font-semibold shadow-lg shadow-indigo-600/20 transition-all">Download</a>';
        } else {
          buttons += '<span class="bg-white/5 text-gray-500 px-3 py-1.5 rounded-lg text-xs font-semibold">Unavailable</span>';
        }
      }

      return '<div onclick="navigate(\'' + targetUrl + '\')" class="group flex items-center justify-between p-4 rounded-2xl hover:bg-white/[0.03] border border-transparent hover:border-white/5 transition-all duration-150 cursor-pointer">' +
        '<div class="flex items-center gap-4 min-w-0 pr-4">' +
          icon +
          '<div class="min-w-0">' +
            '<div class="flex items-center">' +
              '<span class="block text-sm font-semibold text-gray-200 group-hover:text-indigo-300 transition-colors truncate max-w-md md:max-w-xl">' + escapeHtml(item.name) + '</span>' +
              accountBadge +
            '</div>' +
            '<span class="block text-xs text-gray-400 mt-0.5">' + (isFolder ? "Folder" : ("File • " + formatBytes(item.size))) + '</span>' +
          '</div>' +
        '</div>' +
        '<div class="shrink-0 flex items-center">' +
          buttons +
        '</div>' +
      '</div>';
    }).join("");
  }

  const html = '<!DOCTYPE html>' +
'<html lang="en">' +
'<head>' +
'<meta charset="UTF-8">' +
'<meta name="viewport" content="width=device-width, initial-scale=1.0">' +
'<title>MEGA Video & File CDN</title>' +
'<script src="https://cdn.tailwindcss.com"></script>' +
'<link href="https://fonts.googleapis.com/css2?family=Inter:wght@300;400;500;600;700&display=swap" rel="stylesheet">' +
'<style>' +
'body { font-family: "Inter", sans-serif; background: radial-gradient(circle at top left, #0e111d 0%, #07080e 100%); }' +
'.glass { background: rgba(255, 255, 255, 0.02); backdrop-filter: blur(20px); border: 1px solid rgba(255, 255, 255, 0.05); }' +
'</style>' +
'</head>' +
'<body class="min-h-screen text-gray-200 p-4 md:p-8 flex justify-center items-start">' +
'<div class="w-full max-w-5xl flex flex-col gap-6 mt-4">' +
  '<div class="flex flex-col md:flex-row md:items-center justify-between gap-4 glass rounded-3xl p-6 border border-white/5 shadow-2xl">' +
    '<div>' +
      '<h1 class="text-2xl font-bold bg-gradient-to-r from-indigo-400 via-purple-400 to-cyan-400 bg-clip-text text-transparent">MEGA Video CDN</h1>' +
      '<p class="text-xs text-gray-400 mt-1">Anonymous HLS streaming & file index (' + SHARED_FOLDERS.length + ' public pools)</p>' +
    '</div>' +
    '<div class="flex gap-3 flex-wrap">' +
      '<a href="/" class="flex items-center gap-2 bg-white/5 hover:bg-white/10 text-gray-300 hover:text-white font-medium text-sm px-4 py-2.5 rounded-xl border border-white/10 transition-all duration-150">Home</a>' +
      '<button onclick="togglePublicForm()" class="flex items-center gap-2 bg-cyan-600 hover:bg-cyan-500 text-white font-medium text-sm px-4 py-2.5 rounded-xl transition-all duration-150 shadow-lg shadow-cyan-600/20">Index Public Link</button>' +
    '</div>' +
  '</div>' +

  '<div id="publicForm" class="hidden glass rounded-3xl p-6 border border-white/5 shadow-2xl flex-col gap-3">' +
    '<label class="text-sm font-semibold text-gray-300">Paste a public mega.nz link</label>' +
    '<div class="flex flex-col md:flex-row gap-3">' +
      '<input type="text" id="publicLinkInput" placeholder="https://mega.nz/folder/HANDLE#KEY" class="flex-1 bg-black/40 border border-white/10 rounded-xl px-4 py-2.5 text-sm text-gray-200 focus:outline-none focus:border-cyan-500 transition-colors">' +
      '<button onclick="openPublicLink()" class="bg-cyan-600 hover:bg-cyan-500 text-white font-medium text-sm px-5 py-2.5 rounded-xl transition-all">Browse</button>' +
    '</div>' +
  '</div>' +

  '<div class="glass rounded-3xl p-6 border border-white/5 shadow-2xl flex flex-col gap-4">' +
    '<div class="flex items-center overflow-x-auto pb-4 border-b border-white/5 text-sm text-gray-400">' +
      breadcrumbsHtml +
    '</div>' +
    '<div class="flex flex-col" id="itemsList">' +
      itemsHtml +
    '</div>' +
  '</div>' +
'</div>' +
'<script>' +
'function navigate(url) { if (url && url !== "#") window.location.href = url; }' +
'function togglePublicForm() { const el = document.getElementById("publicForm"); el.classList.toggle("hidden"); el.classList.toggle("flex"); }' +
'function openPublicLink() { const val = document.getElementById("publicLinkInput").value.trim(); if (!val) return; window.location.href = "/open?link=" + encodeURIComponent(val); }' +
'</script>' +
'</body>' +
'</html>';

  return new Response(html, { headers: { "Content-Type": "text/html; charset=UTF-8" } });
}

// --- Public Folder Explorer UI ---
function renderUI(tree, currentNodeHash, origin, opts) {
  const node = tree.nodes[currentNodeHash];
  if (!node) return new Response("Folder Not Found", { status: 404 });

  let breadcrumbs = [];
  let curr = node;
  while (curr) {
    breadcrumbs.unshift(curr);
    curr = tree.nodes[curr.parent];
  }

  const breadcrumbsHtml = breadcrumbs.map((b, idx) => {
    const isLast = idx === breadcrumbs.length - 1;
    if (isLast) return '<span class="text-indigo-300 font-semibold truncate max-w-xs">' + escapeHtml(b.name || "Root") + '</span>';
    return '<a href="' + opts.basePath + '?sub=' + b.hash + '" class="hover:text-white transition-colors truncate max-w-[160px]">' + escapeHtml(b.name || "Root") + '</a><span class="text-gray-600 mx-2">/</span>';
  }).join("");

  const refreshUrl = opts.basePath + "?sub=" + currentNodeHash + "&refresh=1";

  let itemsListHtml = "";
  if (!node.children || node.children.length === 0) {
    itemsListHtml = '<div class="text-center py-16 text-gray-500 font-medium">This folder is empty.</div>';
  } else {
    itemsListHtml = node.children.map(child => {
      const isFolder = child.type === 1;
      const isVideo = /\.(mp4|mkv|webm|avi|mov|m4v)$/i.test(child.name);
      const icon = isFolder
        ? '<svg class="w-10 h-10 text-indigo-400" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z"></path></svg>'
        : '<svg class="w-10 h-10 text-cyan-400" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 21h10a2 2 0 002-2V9.414a1 1 0 00-.293-.707l-5.414-5.414A1 1 0 0012.586 3H7a2 2 0 00-2 2v14a2 2 0 002 2z"></path></svg>';
      
      const targetUrl = isFolder
        ? opts.basePath + "?sub=" + child.hash
        : opts.basePath + "/download/" + child.hash;

      let actionButtons = "";
      if (isFolder) {
        actionButtons = '<button class="bg-white/5 hover:bg-white/10 text-gray-300 hover:text-white px-3.5 py-1.5 rounded-lg text-xs font-semibold border border-white/5">Open</button>';
      } else {
        if (isVideo) {
          const playUrl = opts.basePath.replace(/^\/p\//, "/play/") + "/" + child.hash;
          actionButtons += '<a href="' + playUrl + '" class="bg-cyan-600 hover:bg-cyan-500 text-white px-3 py-1.5 rounded-lg text-xs font-semibold mr-2">Play HLS</a>';
        }
        actionButtons += '<a href="' + targetUrl + '" class="bg-indigo-600 hover:bg-indigo-500 text-white px-3 py-1.5 rounded-lg text-xs font-semibold">Download</a>';
      }

      return '<div onclick="navigate(\'' + targetUrl + '\')" class="group flex items-center justify-between p-4 rounded-2xl hover:bg-white/[0.03] transition-colors cursor-pointer">' +
        '<div class="flex items-center gap-4 min-w-0 pr-4">' +
          icon +
          '<div class="min-w-0">' +
            '<span class="block text-sm font-semibold text-gray-200 group-hover:text-indigo-300 transition-colors truncate">' + escapeHtml(child.name) + '</span>' +
            '<span class="block text-xs text-gray-400 mt-0.5">' + (isFolder ? "Folder" : ("File • " + formatBytes(child.size))) + '</span>' +
          '</div>' +
        '</div>' +
        '<div class="shrink-0 flex items-center">' + actionButtons + '</div>' +
      '</div>';
    }).join("");
  }

  const html = '<!DOCTYPE html><html lang="en"><head><meta charset="UTF-8"><title>' + escapeHtml(opts.title) + '</title>' +
    '<script src="https://cdn.tailwindcss.com"></script><link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;600;700&display=swap" rel="stylesheet">' +
    '<style>body{font-family:"Inter",sans-serif;background:radial-gradient(circle at top left,#0e111d 0%,#07080e 100%)}.glass{background:rgba(255,255,255,0.02);backdrop-filter:blur(20px);border:1px solid rgba(255,255,255,0.05)}</style></head>' +
    '<body class="min-h-screen text-gray-200 p-4 md:p-8 flex justify-center items-start"><div class="w-full max-w-5xl flex flex-col gap-6 mt-4">' +
    '<div class="flex flex-col md:flex-row md:items-center justify-between gap-4 glass rounded-3xl p-6 border border-white/5 shadow-2xl">' +
    '<div><h1 class="text-2xl font-bold bg-gradient-to-r from-indigo-400 to-cyan-400 bg-clip-text text-transparent">' + escapeHtml(opts.title) + '</h1><p class="text-xs text-gray-400 mt-1">' + escapeHtml(opts.subtitle) + '</p></div>' +
    '<div class="flex gap-3"><a href="/" class="bg-white/5 hover:bg-white/10 px-4 py-2.5 rounded-xl text-sm font-medium">Home</a><a href="' + refreshUrl + '" class="bg-indigo-600 hover:bg-indigo-500 px-4 py-2.5 rounded-xl text-sm font-medium">Refresh</a></div></div>' +
    '<div class="glass rounded-3xl p-6 border border-white/5 shadow-2xl flex flex-col gap-4">' +
    '<div class="flex items-center overflow-x-auto pb-4 border-b border-white/5">' + breadcrumbsHtml + '</div>' +
    '<div class="flex flex-col" id="itemsList">' + itemsListHtml + '</div></div></div>' +
    '<script>function navigate(u){if(u)window.location.href=u;}</script></body></html>';

  return new Response(html, { headers: { "Content-Type": "text/html; charset=UTF-8" } });
}

function renderFileUI(meta, basePath) {
  const downloadHref = basePath + "/download";
  const sizeStr = formatBytes(meta.size);
  const html = '<!DOCTYPE html><html lang="en"><head><meta charset="UTF-8"><title>' + escapeHtml(meta.name) + '</title>' +
    '<script src="https://cdn.tailwindcss.com"></script><style>body{font-family:sans-serif;background:#0e111d;color:#e2e8f0}.glass{background:rgba(255,255,255,0.02);border:1px solid rgba(255,255,255,0.05)}</style></head>' +
    '<body class="min-h-screen p-8 flex justify-center items-center"><div class="glass rounded-3xl p-8 max-w-xl w-full flex flex-col gap-4">' +
    '<h1 class="text-xl font-bold text-cyan-400">' + escapeHtml(meta.name) + '</h1><p class="text-sm text-gray-400">' + sizeStr + '</p>' +
    '<div class="flex gap-3 mt-4"><a href="' + downloadHref + '" class="bg-indigo-600 hover:bg-indigo-500 text-white px-6 py-2.5 rounded-xl text-sm font-semibold">Download Direct</a>' +
    '<a href="/" class="bg-white/5 hover:bg-white/10 px-4 py-2.5 rounded-xl text-sm font-medium">Home</a></div></div></body></html>';
  return new Response(html, { headers: { "Content-Type": "text/html; charset=UTF-8" } });
}

// --- MEGA Cryptographic & Streaming Engine ---

function parseMegaPublicLink(link) {
  try {
    link = link.trim();
    let m = link.match(/mega(?:\.co)?\.nz\/(folder|file)\/([a-zA-Z0-9_-]+)#([a-zA-Z0-9_-]+)/);
    if (m) return { type: m[1], handle: m[2], key: m[3] };
    m = link.match(/mega(?:\.co)?\.nz\/#F!([a-zA-Z0-9_-]+)!([a-zA-Z0-9_-]+)/);
    if (m) return { type: "folder", handle: m[1], key: m[2] };
    m = link.match(/mega(?:\.co)?\.nz\/#!([a-zA-Z0-9_-]+)!([a-zA-Z0-9_-]+)/);
    if (m) return { type: "file", handle: m[1], key: m[2] };
    return null;
  } catch {
    return null;
  }
}

async function getOrFetchPublicTree(folderHandle, keyB64, forceRefresh) {
  const cached = publicTreeCache[folderHandle];
  if (cached && !forceRefresh) return cached;
  const tree = await fetchPublicFolderTree(folderHandle, keyB64);
  publicTreeCache[folderHandle] = tree;
  return tree;
}

async function fetchPublicFolderTree(folderHandle, keyB64) {
  const shareKey = decodeBase64(keyB64);
  const sn = Math.floor(Math.random() * 2147483647);
  const response = await fetch("https://g.api.mega.co.nz/cs?id=" + sn + "&n=" + folderHandle, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify([{ a: "f", c: 1, r: 1 }])
  });
  if (!response.ok) throw new Error("MEGA API fetch failed: " + response.status);
  const data = await response.json();
  const rawNodes = data[0].f || [];
  const decryptedNodes = {};
  for (const rawNode of rawNodes) {
    if (rawNode.t !== 0 && rawNode.t !== 1) continue;
    const node = await decryptPublicNode(rawNode, shareKey);
    if (node) {
      node.children = [];
      decryptedNodes[node.hash] = node;
    }
  }
  let rootNodeHash = null;
  for (const hash in decryptedNodes) {
    const node = decryptedNodes[hash];
    if (node.parent && decryptedNodes[node.parent]) {
      decryptedNodes[node.parent].children.push(node);
    } else if (rootNodeHash === null || node.type === 1) {
      if (!decryptedNodes[node.parent] && rootNodeHash === null) rootNodeHash = node.hash;
    }
  }
  return { nodes: decryptedNodes, rootHash: rootNodeHash || folderHandle };
}

async function decryptPublicNode(itm, shareKey) {
  try {
    const parts = itm.k.split(":");
    if (parts.length < 2) return null;
    const itemKeyBase64 = parts[1].split("/")[0];
    const encryptedKeyBytes = decodeBase64(itemKeyBase64);
    const decryptedKeyBytes = decryptAesEcbJS(shareKey, encryptedKeyBytes);

    let key, iv = null;
    if (itm.t === 0) {
      key = new Uint8Array(16);
      for (let i = 0; i < 16; i++) key[i] = decryptedKeyBytes[i] ^ decryptedKeyBytes[i + 16];
      iv = new Uint8Array(16);
      iv.set(decryptedKeyBytes.subarray(16, 24), 0);
    } else {
      key = decryptedKeyBytes;
    }

    const attr = await decryptAttr(key, itm.a);
    return { hash: itm.h, parent: itm.p, type: itm.t, name: attr.n, size: itm.s || 0, ts: itm.ts, key, iv };
  } catch {
    return null;
  }
}

async function handlePublicFolderDownload(request, folderHandle, keyB64, nodeHash) {
  const tree = await getOrFetchPublicTree(folderHandle, keyB64, false);
  const node = tree.nodes[nodeHash];
  if (!node || node.type !== 0) return new Response("File Not Found", { status: 404 });
  const sn = Math.floor(Math.random() * 2147483647);
  const apiRes = await fetch("https://g.api.mega.co.nz/cs?id=" + sn + "&n=" + folderHandle, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify([{ a: "g", g: 1, n: nodeHash }])
  });
  const apiData = await apiRes.json();
  return streamMegaFile(request, { size: node.size || 0, name: node.name, key: node.key, iv: node.iv }, apiData[0].g);
}

async function handlePublicFileDownload(request, fileHandle, keyB64) {
  const keyBytes = decodeBase64(keyB64);
  const key = new Uint8Array(16);
  for (let i = 0; i < 16; i++) key[i] = keyBytes[i] ^ keyBytes[i + 16];
  const iv = new Uint8Array(16);
  iv.set(keyBytes.subarray(16, 24), 0);

  const sn = Math.floor(Math.random() * 2147483647);
  const apiRes = await fetch("https://g.api.mega.co.nz/cs?id=" + sn, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify([{ a: "g", g: 1, p: fileHandle }])
  });
  const apiData = await apiRes.json();
  let name = "download.bin";
  try {
    const attr = await decryptAttr(key, apiData[0].at);
    if (attr && attr.n) name = attr.n;
  } catch {}
  return streamMegaFile(request, { size: apiData[0].s || 0, name, key, iv }, apiData[0].g);
}

async function fetchPublicFileMeta(fileHandle, keyB64) {
  const keyBytes = decodeBase64(keyB64);
  const key = new Uint8Array(16);
  for (let i = 0; i < 16; i++) key[i] = keyBytes[i] ^ keyBytes[i + 16];
  const sn = Math.floor(Math.random() * 2147483647);
  const apiRes = await fetch("https://g.api.mega.co.nz/cs?id=" + sn, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify([{ a: "g", p: fileHandle }])
  });
  const apiData = await apiRes.json();
  let name = "download.bin";
  try {
    const attr = await decryptAttr(key, apiData[0].at);
    if (attr && attr.n) name = attr.n;
  } catch {}
  return { name, size: apiData[0].s || 0 };
}

async function streamMegaFile(request, node, downloadUrl) {
  const fileSize = node.size || 0;
  const rangeHeader = request.headers.get("Range");
  let isRange = false, rangeStart = 0, rangeEnd = fileSize - 1;

  if (rangeHeader && rangeHeader.startsWith("bytes=")) {
    const firstRange = rangeHeader.substring(6).split(",")[0].trim();
    const parts = firstRange.split("-");
    if (parts[0].trim() !== "") {
      rangeStart = parseInt(parts[0].trim(), 10);
      isRange = true;
      if (parts[1] && parts[1].trim() !== "") rangeEnd = parseInt(parts[1].trim(), 10);
    }
  }

  if (rangeEnd >= fileSize) rangeEnd = fileSize - 1;
  const aesStart = isRange ? Math.floor(rangeStart / 16) * 16 : 0;
  let discardBytes = isRange ? rangeStart - aesStart : 0;

  const cryptoKey = await crypto.subtle.importKey("raw", node.key, { name: "AES-CTR" }, false, ["decrypt"]);
  const expectedSize = rangeEnd - rangeStart + 1;
  const CHUNK = 32 * 1024 * 1024;
  let pos = aesStart;
  const end = rangeEnd;

  const stream = new ReadableStream({
    async pull(controller) {
      if (pos > end) { controller.close(); return; }
      const chunkEnd = Math.min(pos + CHUNK - 1, end);
      try {
        const chunkRes = await fetch(downloadUrl, { headers: { "Range": "bytes=" + pos + "-" + chunkEnd } });
        const encrypted = new Uint8Array(await chunkRes.arrayBuffer());
        const iv = getCtrIv(node.iv, pos);
        let decrypted = new Uint8Array(await crypto.subtle.decrypt({ name: "AES-CTR", counter: iv, length: 64 }, cryptoKey, encrypted));
        if (discardBytes > 0) {
          decrypted = decrypted.slice(discardBytes);
          discardBytes = 0;
        }
        controller.enqueue(decrypted);
        pos = chunkEnd + 1;
      } catch (err) {
        controller.error(err);
      }
    }
  });

  const { readable, writable } = new FixedLengthStream(expectedSize);
  stream.pipeTo(writable).catch(() => {});

  const headers = new Headers({
    "Content-Type": "application/octet-stream",
    "Content-Disposition": 'attachment; filename="' + encodeURIComponent(node.name) + '"',
    "Accept-Ranges": "bytes",
    "Content-Length": expectedSize.toString()
  });
  let status = isRange ? 206 : 200;
  if (isRange) headers.set("Content-Range", "bytes " + rangeStart + "-" + rangeEnd + "/" + fileSize);
  return new Response(readable, { status, headers });
}

function getCtrIv(metaIv, chkStart) {
  const iv = new Uint8Array(16);
  iv.set(metaIv.subarray(0, 8), 0);
  const counter = BigInt(chkStart) / 16n;
  const view = new DataView(iv.buffer);
  view.setBigUint64(8, counter, false);
  return iv;
}

const S = [0x63, 0x7c, 0x77, 0x7b, 0xf2, 0x6b, 0x6f, 0xc5, 0x30, 0x01, 0x67, 0x2b, 0xfe, 0xd7, 0xab, 0x76, 0xca, 0x82, 0xc9, 0x7d, 0xfa, 0x59, 0x47, 0xf0, 0xad, 0xd4, 0xa2, 0xaf, 0x9c, 0xa4, 0x72, 0xc0, 0xb7, 0xfd, 0x93, 0x26, 0x36, 0x3f, 0xf7, 0xcc, 0x34, 0xa5, 0xe5, 0xf1, 0x71, 0xd8, 0x31, 0x15, 0x04, 0xc7, 0x23, 0xc3, 0x18, 0x96, 0x05, 0x9a, 0x07, 0x12, 0x80, 0xe2, 0xeb, 0x27, 0xb2, 0x75, 0x09, 0x83, 0x2c, 0x1a, 0x1b, 0x6e, 0x5a, 0xa0, 0x52, 0x3b, 0xd6, 0xb3, 0x29, 0xe3, 0x2f, 0x84, 0x53, 0xd1, 0x00, 0xed, 0x20, 0xfc, 0xb1, 0x5b, 0x6a, 0xcb, 0xbe, 0x39, 0x4a, 0x4c, 0x58, 0xcf, 0xd0, 0xef, 0xaa, 0xfb, 0x43, 0x4d, 0x33, 0x85, 0x45, 0xf9, 0x02, 0x7f, 0x50, 0x3c, 0x9f, 0xa8, 0x51, 0xa3, 0x40, 0x8f, 0x92, 0x9d, 0x38, 0xf5, 0xbc, 0xb6, 0xda, 0x21, 0x10, 0xff, 0xf3, 0xd2, 0xcd, 0x0c, 0x13, 0xec, 0x5f, 0x97, 0x44, 0x17, 0xc4, 0xa7, 0x7e, 0x3d, 0x64, 0x5d, 0x19, 0x73, 0x60, 0x81, 0x4f, 0xdc, 0x22, 0x2a, 0x90, 0x88, 0x46, 0xee, 0xb8, 0x14, 0xde, 0x5e, 0x0b, 0xdb, 0xe0, 0x32, 0x3a, 0x0a, 0x49, 0x06, 0x24, 0x5c, 0xc2, 0xd3, 0xac, 0x62, 0x91, 0x95, 0xe4, 0x79, 0xe7, 0xc8, 0x37, 0x6d, 0x8d, 0xd5, 0x4e, 0xa9, 0x6c, 0x56, 0xf4, 0xea, 0x65, 0x7a, 0xae, 0x08, 0xba, 0x78, 0x25, 0x2e, 0x1c, 0xa6, 0xb4, 0xc6, 0xe8, 0xdd, 0x74, 0x1f, 0x4b, 0xbd, 0x8b, 0x8a, 0x70, 0x3e, 0xb5, 0x66, 0x48, 0x03, 0xf6, 0x0e, 0x61, 0x35, 0x57, 0xb9, 0x86, 0xc1, 0x1d, 0x9e, 0xe1, 0xf8, 0x98, 0x11, 0x69, 0xd9, 0x8e, 0x94, 0x9b, 0x1e, 0x87, 0xe9, 0xce, 0x55, 0x28, 0xdf, 0x8c, 0xa1, 0x89, 0x0d, 0xbf, 0xe6, 0x42, 0x68, 0x41, 0x99, 0x2d, 0x0f, 0xb0, 0x54, 0xbb, 0x16];
const Si = [0x52, 0x09, 0x6a, 0xd5, 0x30, 0x36, 0xa5, 0x38, 0xbf, 0x40, 0xa3, 0x9e, 0x81, 0xf3, 0xd7, 0xfb, 0x7c, 0xe3, 0x39, 0x82, 0x9b, 0x2f, 0xff, 0x87, 0x34, 0x8e, 0x43, 0x44, 0xc4, 0xde, 0xe9, 0xcb, 0x54, 0x7b, 0x94, 0x32, 0xa6, 0xc2, 0x23, 0x3d, 0xee, 0x4c, 0x95, 0x0b, 0x42, 0xfa, 0xc3, 0x4e, 0x08, 0x2e, 0xa1, 0x66, 0x28, 0xd9, 0x24, 0xb2, 0x76, 0x5b, 0xa2, 0x49, 0x6d, 0x8b, 0xd1, 0x25, 0x72, 0xf8, 0xf6, 0x64, 0x86, 0x68, 0x98, 0x16, 0xd4, 0xa4, 0x5c, 0xcc, 0x5d, 0x65, 0xb6, 0x92, 0x6c, 0x70, 0x48, 0x50, 0xfd, 0xed, 0xb9, 0xda, 0x5e, 0x15, 0x46, 0x57, 0xa7, 0x8d, 0x9d, 0x84, 0x90, 0xd8, 0xab, 0x00, 0x8c, 0xbc, 0xd3, 0x0a, 0xf7, 0xe4, 0x58, 0x05, 0xb8, 0xb3, 0x45, 0x06, 0xd0, 0x2c, 0x1e, 0x8f, 0xca, 0x3f, 0x0f, 0x02, 0xc1, 0xaf, 0xbd, 0x03, 0x01, 0x13, 0x8a, 0x6b, 0x3a, 0x91, 0x11, 0x41, 0x4f, 0x67, 0xdc, 0xea, 0x97, 0xf2, 0xcf, 0xce, 0xf0, 0xb4, 0xe6, 0x73, 0x96, 0xac, 0x74, 0x22, 0xe7, 0xad, 0x35, 0x85, 0xe2, 0xf9, 0x37, 0xe8, 0x1c, 0x75, 0xdf, 0x6e, 0x47, 0xf1, 0x1a, 0x71, 0x1d, 0x29, 0xc5, 0x89, 0x6f, 0xb7, 0x62, 0x0e, 0xaa, 0x18, 0xbe, 0x1b, 0xfc, 0x56, 0x3e, 0x4b, 0xc6, 0xd2, 0x79, 0x20, 0x9a, 0xdb, 0xc0, 0xfe, 0x78, 0xcd, 0x5a, 0xf4, 0x1f, 0xdd, 0xa8, 0x33, 0x88, 0x07, 0xc7, 0x31, 0xb1, 0x12, 0x10, 0x59, 0x27, 0x80, 0xec, 0x5f, 0x60, 0x51, 0x7f, 0xa9, 0x19, 0xb5, 0x4a, 0x0d, 0x2d, 0xe5, 0x7a, 0x9f, 0x93, 0xc9, 0x9c, 0xef, 0xa0, 0xe0, 0x3b, 0x4d, 0xae, 0x2a, 0xf5, 0xb0, 0xc8, 0xeb, 0xbb, 0x3c, 0x83, 0x53, 0x99, 0x61, 0x17, 0x2b, 0x04, 0x7e, 0xba, 0x77, 0xd6, 0x26, 0xe1, 0x69, 0x14, 0x63, 0x55, 0x21, 0x0c, 0x7d];
const T5 = [0x51f4a750, 0x7e416553, 0x1a17a4c3, 0x3a275e96, 0x3bab6bcb, 0x1f9d45f1, 0xacfa58ab, 0x4be30393, 0x2030fa55, 0xad766df6, 0x88cc7691, 0xf5024c25, 0x4fe5d7fc, 0xc52acbd7, 0x26354480, 0xb562a38f, 0xdeb15a49, 0x25ba1b67, 0x45ea0e98, 0x5dfec0e1, 0xc32f7502, 0x814cf012, 0x8d4697a3, 0x6bd3f9c6, 0x038f5fe7, 0x15929c95, 0xbf6d7aeb, 0x955259da, 0xd4be832d, 0x587421d3, 0x49e06929, 0x8ec9c844, 0x75c2896a, 0xf48e7978, 0x99583e6b, 0x27b971dd, 0xbee14fb6, 0xf088ad17, 0xc920ac66, 0x7dce3ab4, 0x63df4a18, 0xe51a3182, 0x97513360, 0x62537f45, 0xb16477e0, 0xbb6bae84, 0xfe81a01c, 0xf9082b94, 0x70486858, 0x8f45fd19, 0x94de6c87, 0x527bf8b7, 0xab73d323, 0x724b02e2, 0xe31f8f57, 0x6655ab2a, 0xb2eb2807, 0x2fb5c203, 0x86c57b9a, 0xd33708a5, 0x302887f2, 0x23bfa5b2, 0x02036aba, 0xed16825c, 0x8acf1c2b, 0xa779b492, 0xf307f2f0, 0x4e69e2a1, 0x65daf4cd, 0x0605bed5, 0xd134621f, 0xc4a6fe8a, 0x342e539d, 0xa2f355a0, 0x058ae132, 0xa4f6eb75, 0x0b83ec39, 0x4060efaa, 0x5e719f06, 0xbd6e1051, 0x3e218af9, 0x96dd063d, 0xdd3e05ae, 0x4de6bd46, 0x91548db5, 0x71c45d05, 0x0406d46f, 0x605015ff, 0x1998fb24, 0xd6bde997, 0x894043cc, 0x67d99e77, 0xb0e842bd, 0x07898b88, 0xe7195b38, 0x79c8eedb, 0xa17c0a47, 0x7c420fe9, 0xf8841ec9, 0x00000000, 0x09808683, 0x322bed48, 0x1e1170ac, 0x6c5a724e, 0xfd0efffb, 0x0f853856, 0x3daed51e, 0x362d3927, 0x0a0fd964, 0x685ca621, 0x9b5b54d1, 0x24362e3a, 0x0c0a67b1, 0x9357e70f, 0xb4ee96d2, 0x1b9b919e, 0x80c0c54f, 0x61dc20a2, 0x5a774b69, 0x1c121a16, 0xe293ba0a, 0xc0a02ae5, 0x3c22e043, 0x121b171d, 0x0e090d0b, 0xf28bc7ad, 0x2db6a8b9, 0x141ea9c8, 0x57f11985, 0xaf75074c, 0xee99ddbb, 0xa37f60fd, 0xf701269f, 0x5c72f5bc, 0x44663bc5, 0x5bfb7e34, 0x8b432976, 0xcb23c6dc, 0xb6edfc68, 0xb8e4f163, 0xd731dcca, 0x42638510, 0x13972240, 0x84c61120, 0x854a247d, 0xd2bb3df8, 0xaef93211, 0xc729a16d, 0x1d9e2f4b, 0xdcb230f3, 0x0d8652ec, 0x77c1e3d0, 0x2bb3166c, 0xa970b999, 0x119448fa, 0x47e96422, 0xa8fc8cc4, 0xa0f03f1a, 0x567d2cd8, 0x223390ef, 0x87494ec7, 0xd938d1c1, 0x8ccaa2fe, 0x98d40b36, 0xa6f581cf, 0xa57ade28, 0xdab78e26, 0x3fadbfa4, 0x2c3a9de4, 0x5078920d, 0x6a5fcc9b, 0x547e4662, 0xf68d13c2, 0x90d8b8e8, 0x2e39f75e, 0x82c3aff5, 0x9f5d80be, 0x69d0937c, 0x6fd52da9, 0xcf2512b3, 0xc8ac993b, 0x10187da7, 0xe89c636e, 0xdb3bbb7b, 0xcd267809, 0x6e5918f4, 0xec9ab701, 0x834f9aa8, 0xe6956e65, 0xaaffe67e, 0x21bccf08, 0xef15e8e6, 0xbae79bd9, 0x4a6f36ce, 0xea9f09d4, 0x29b07cd6, 0x31a4b2af, 0x2a3f2331, 0xc6a59430, 0x35a266c0, 0x744ebc37, 0xfc82caa6, 0xe090d0b0, 0x33a7d815, 0xf104984a, 0x41ecdaf7, 0x7fcd500e, 0x1791f62f, 0x764dd68d, 0x43efb04d, 0xccaa4d54, 0xe49604df, 0x9ed1b5e3, 0x4c6a881b, 0xc12c1fb8, 0x4665517f, 0x9d5eea04, 0x018c355d, 0xfa877473, 0xfb0b412e, 0xb3671d5a, 0x92dbd252, 0xe9105633, 0x6dd64713, 0x9ad7618c, 0x37a10c7a, 0x59f8148e, 0xeb133c89, 0xcea927ee, 0xb761c935, 0xe11ce5ed, 0x7a47b13c, 0x9cd2df59, 0x55f2733f, 0x1814ce79, 0x73c737bf, 0x53f7cdea, 0x5ffdaa5b, 0xdf3d6f14, 0x7844db86, 0xcaaff381, 0xb968c43e, 0x3824342c, 0xc2a3405f, 0x161dc372, 0xbce2250c, 0x283c498b, 0xff0d9541, 0x39a80171, 0x080cb3de, 0xd8b4e49c, 0x6456c190, 0x7bcb8461, 0xd532b670, 0x486c5c74, 0xd0b85742];

const T6 = T5.map(v => ((v >>> 8) | ((v & 0xff) << 24)) >>> 0);
const T7 = T5.map(v => ((v >>> 16) | ((v & 0xffff) << 16)) >>> 0);
const T8 = T5.map(v => ((v >>> 24) | ((v & 0xffffff) << 8)) >>> 0);

function convertToInt32(bytes) {
  var result = [];
  for (var i = 0; i < bytes.length; i += 4) {
    result.push(
      (bytes[i] << 24) |
      (bytes[i + 1] << 16) |
      (bytes[i + 2] << 8) |
      bytes[i + 3]
    );
  }
  return result;
}

var AES = function(key) {
  this.key = key;
  this._prepare();
};

AES.prototype._prepare = function() {
  this._Kd = [];
  var rounds = 10;
  var roundKeyCount = (rounds + 1) * 4;
  var KC = 4;
  for (var i = 0; i <= rounds; i++) {
    this._Kd.push([0, 0, 0, 0]);
  }
  var tk = convertToInt32(this.key);
  for (var i = 0; i < KC; i++) {
    var index = i >> 2;
    this._Kd[rounds - index][i % 4] = tk[i];
  }
  var rconpointer = 0;
  var t = KC, tt;
  var rcon = [0x01, 0x02, 0x04, 0x08, 0x10, 0x20, 0x40, 0x80, 0x1b, 0x36];
  while (t < roundKeyCount) {
    tt = tk[KC - 1];
    tk[0] ^= ((S[(tt >> 16) & 0xFF] << 24) ^
      (S[(tt >> 8) & 0xFF] << 16) ^
      (S[tt & 0xFF] << 8) ^
      S[(tt >> 24) & 0xFF] ^
      (rcon[rconpointer] << 24));
    rconpointer += 1;
    for (var i = 1; i < KC; i++) {
      tk[i] ^= tk[i - 1];
    }
    var destRound = (t >> 2);
    this._Kd[rounds - destRound][t % 4] = tk[0];
    this._Kd[rounds - destRound][(t + 1) % 4] = tk[1];
    this._Kd[rounds - destRound][(t + 2) % 4] = tk[2];
    this._Kd[rounds - destRound][(t + 3) % 4] = tk[3];
    t += 4;
  }
  var roundKey;
  for (var r = 1; r < rounds; r++) {
    roundKey = this._Kd[r];
    for (var i = 0; i < 4; i++) {
      tt = roundKey[i];
      roundKey[i] = (T5[S[(tt >> 24) & 0xFF]] ^
        T6[S[(tt >> 16) & 0xFF]] ^
        T7[S[(tt >> 8) & 0xFF]] ^
        T8[S[tt & 0xFF]]);
    }
  }
};

AES.prototype.decrypt = function(ciphertext) {
  var rounds = 10;
  var a = [0, 0, 0, 0];
  var t = convertToInt32(ciphertext);
  for (var i = 0; i < 4; i++) {
    t[i] ^= this._Kd[0][i];
  }
  for (var r = 1; r < rounds; r++) {
    for (var i = 0; i < 4; i++) {
      a[i] = (T5[(t[i] >> 24) & 0xff] ^
        T6[(t[(i + 3) % 4] >> 16) & 0xff] ^
        T7[(t[(i + 2) % 4] >> 8) & 0xff] ^
        T8[t[(i + 1) % 4] & 0xff] ^
        this._Kd[r][i]);
    }
    t = a.slice();
  }
  var result = new Uint8Array(16);
  for (var i = 0; i < 4; i++) {
    var tt = this._Kd[rounds][i];
    result[4 * i] = (Si[(t[i] >> 24) & 0xff] ^ (tt >> 24)) & 0xff;
    result[4 * i + 1] = (Si[(t[(i + 3) % 4] >> 16) & 0xff] ^ (tt >> 16)) & 0xff;
    result[4 * i + 2] = (Si[(t[(i + 2) % 4] >> 8) & 0xff] ^ (tt >> 8)) & 0xff;
    result[4 * i + 3] = (Si[t[(i + 1) % 4] & 0xff] ^ tt) & 0xff;
  }
  return result;
};

function decryptAesEcbJS(keyBytes, ciphertextBytes) {
  const aes = new AES(keyBytes);
  const result = new Uint8Array(ciphertextBytes.length);
  for (let i = 0; i < ciphertextBytes.length; i += 16) {
    const block = ciphertextBytes.subarray(i, i + 16);
    const decrypted = aes.decrypt(block);
    result.set(decrypted, i);
  }
  return result;
}

function decryptAesCbcJS(keyBytes, ciphertextBytes, iv) {
  const aes = new AES(keyBytes);
  const result = new Uint8Array(ciphertextBytes.length);
  let lastBlock = iv || new Uint8Array(16);
  for (let i = 0; i < ciphertextBytes.length; i += 16) {
    const block = ciphertextBytes.subarray(i, i + 16);
    const decrypted = aes.decrypt(block);
    for (let j = 0; j < 16; j++) {
      result[i + j] = decrypted[j] ^ lastBlock[j];
    }
    lastBlock = block;
  }
  return result;
}

async function decryptAttr(bkey, encryptedAttrB64) {
  const data = decodeBase64(encryptedAttrB64);
  const decrypted = decryptAesCbcJS(bkey, data, new Uint8Array(16));
  const bytes = new Uint8Array(decrypted);
  if (bytes[0] === 77 && bytes[1] === 69 && bytes[2] === 71 && bytes[3] === 65) {
    const decoder = new TextDecoder();
    let jsonStr = decoder.decode(bytes.subarray(4));
    jsonStr = jsonStr.replace(/\0/g, "");
    const startIdx = jsonStr.indexOf("{");
    const endIdx = jsonStr.lastIndexOf("}");
    if (startIdx !== -1 && endIdx !== -1) {
      jsonStr = jsonStr.substring(startIdx, endIdx + 1);
    }
    return JSON.parse(jsonStr);
  }
  throw new Error("Invalid attribute envelope signature");
}

function decodeBase64(str) {
  str = str.replace(/-/g, "+").replace(/_/g, "/");
  while (str.length % 4) {
    str += "=";
  }
  const binary = atob(str);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) {
    bytes[i] = binary.charCodeAt(i);
  }
  return bytes;
}

function formatBytes(bytes, decimals = 2) {
  if (!bytes || bytes === 0) return "0 Bytes";
  const k = 1024;
  const dm = decimals < 0 ? 0 : decimals;
  const sizes = ["Bytes", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(dm)) + " " + sizes[i];
}

function escapeHtml(text) {
  if (!text) return "";
  return text
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#039;");
}
`
}
