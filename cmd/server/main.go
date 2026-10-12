package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/109isaque10/mega-account-generator/pkg/config"
	"github.com/109isaque10/mega-account-generator/pkg/i18n"
	"github.com/109isaque10/mega-account-generator/pkg/mailgun"
	"github.com/109isaque10/mega-account-generator/pkg/mega"
	"github.com/109isaque10/mega-account-generator/pkg/rclone"
	"github.com/109isaque10/mega-account-generator/pkg/scheduler"
	"github.com/109isaque10/mega-account-generator/pkg/storage"
	"github.com/109isaque10/mega-account-generator/pkg/worker"
)

//go:embed web/*
var webFS embed.FS

type LogItem struct {
	Time  string `json:"time"`
	Level string `json:"level"`
	Msg   string `json:"msg"`
}

type GenProgress struct {
	sync.Mutex
	Active       bool   `json:"active"`
	Total        int    `json:"total"`
	Completed    int    `json:"completed"`
	CurrentEmail string `json:"current_email"`
	Status       string `json:"status"` // "idle", "generating", "done"
}

var (
	logMu       sync.RWMutex
	logRing     []LogItem
	genProgress GenProgress
)

func addLog(level, msg string) {
	logMu.Lock()
	defer logMu.Unlock()
	entry := LogItem{
		Time:  time.Now().Format("15:04:05"),
		Level: level,
		Msg:   msg,
	}
	logRing = append(logRing, entry)
	if len(logRing) > 150 {
		logRing = logRing[len(logRing)-150:]
	}
	log.Printf("[%s] %s", strings.ToUpper(level), msg)
}

func main() {
	cfg := config.Load()

	store, err := storage.New(cfg.CsvPath, cfg.JsonPath)
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}

	megaClient := mega.NewClient(cfg.MegatoolsPath)

	sched := scheduler.New(store, megaClient, cfg.SigninIntervalDays, "en", addLog)
	sched.Start()
	defer sched.Stop()

	addLog("info", fmt.Sprintf("Service started on port %s. Pattern: '%s@%s'", cfg.Port, cfg.EmailPattern, cfg.EmailDomain))

	// Embedded Web Assets FS
	subFS, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Fatalf("Failed to load embedded web assets: %v", err)
	}
	fileServer := http.FileServer(http.FS(subFS))

	mux := http.NewServeMux()

	// Static web assets
	mux.Handle("/", fileServer)

	// API Status
	mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, r *http.Request) {
		total, active, pending, failed := store.Count()
		lastRun, nextRun, _ := sched.GetStatus()

		nextIdx := store.GetNextSequentialIndex(cfg.EmailPattern + "@" + cfg.EmailDomain)
		nextEmail := fmt.Sprintf(cfg.EmailPattern+"@"+cfg.EmailDomain, nextIdx)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"total":         total,
			"active":        active,
			"pending":       pending,
			"failed":        failed,
			"pattern":       cfg.EmailPattern,
			"domain":        cfg.EmailDomain,
			"interval_days": cfg.SigninIntervalDays,
			"last_run":      lastRun,
			"next_run":      nextRun,
			"next_email":    nextEmail,
		})
	})

	// API Accounts list
	mux.HandleFunc("GET /api/accounts", func(w http.ResponseWriter, r *http.Request) {
		list := store.List()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(list)
	})

	// Export CSV
	mux.HandleFunc("GET /api/accounts/export.csv", func(w http.ResponseWriter, r *http.Request) {
		file, err := os.Open(cfg.CsvPath)
		if err != nil {
			http.Error(w, "accounts.csv not found", http.StatusNotFound)
			return
		}
		defer file.Close()
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", "attachment; filename=\"accounts.csv\"")
		_, _ = io.Copy(w, file)
	})

	// Export JSON
	mux.HandleFunc("GET /api/accounts/export.json", func(w http.ResponseWriter, r *http.Request) {
		list := store.List()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", "attachment; filename=\"accounts.json\"")
		_ = json.NewEncoder(w).Encode(list)
	})

	// API Logs
	mux.HandleFunc("GET /api/logs", func(w http.ResponseWriter, r *http.Request) {
		logMu.RLock()
		defer logMu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(logRing)
	})

	// API i18n
	mux.HandleFunc("GET /api/i18n", func(w http.ResponseWriter, r *http.Request) {
		lang := r.URL.Query().Get("lang")
		dict := i18n.GetDictionary(lang)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(dict)
	})

	// API Progress of Generator
	mux.HandleFunc("GET /api/accounts/generate/progress", func(w http.ResponseWriter, r *http.Request) {
		genProgress.Lock()
		defer genProgress.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"active":        genProgress.Active,
			"total":         genProgress.Total,
			"completed":     genProgress.Completed,
			"current_email": genProgress.CurrentEmail,
			"status":        genProgress.Status,
		})
	})

	// API Generate Accounts
	mux.HandleFunc("POST /api/accounts/generate", func(w http.ResponseWriter, r *http.Request) {
		genProgress.Lock()
		if genProgress.Active {
			genProgress.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": i18n.T("en", "err_generation_in_progress"),
			})
			return
		}

		var req struct {
			Count    int    `json:"count"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			req.Count = 1
		}
		if req.Count <= 0 || req.Count > 50 {
			req.Count = 1
		}

		genProgress.Active = true
		genProgress.Total = req.Count
		genProgress.Completed = 0
		genProgress.Status = "generating"
		genProgress.Unlock()

		go func() {
			defer func() {
				genProgress.Lock()
				genProgress.Active = false
				genProgress.Status = "idle"
				genProgress.Unlock()
			}()

			for i := 0; i < req.Count; i++ {
				nextIdx := store.GetNextSequentialIndex(cfg.EmailPattern + "@" + cfg.EmailDomain)
				email := fmt.Sprintf(cfg.EmailPattern+"@"+cfg.EmailDomain, nextIdx)
				password := req.Password
				if password == "" {
					if cfg.DefaultPassword != "" {
						password = cfg.DefaultPassword
					} else {
						password = mega.GenerateRandomPassword(14)
					}
				}
				name := mega.GenerateRandomName()

				genProgress.Lock()
				genProgress.Completed = i
				genProgress.CurrentEmail = email
				genProgress.Unlock()

				addLog("info", i18n.T("en", "msg_account_registered", email))

				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				verifyCmd, err := megaClient.Register(ctx, email, name, password)
				cancel()

				if err != nil {
					addLog("error", i18n.T("en", "msg_account_failed", email, err.Error()))
					continue
				}

				acc := &storage.Account{
					Email:         email,
					MegaPassword:  password,
					VerifyCommand: verifyCmd,
					Status:        "pending",
					CreatedAt:     time.Now(),
				}
				_ = store.SavePending(acc)
				addLog("info", fmt.Sprintf("Awaiting Mailgun webhook for %s", email))

				genProgress.Lock()
				genProgress.Completed = i + 1
				genProgress.Unlock()

				// Brief pause between account creations
				time.Sleep(1 * time.Second)
			}
		}()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status": "started",
		})
	})

	// Export rclone.conf
	mux.HandleFunc("GET /api/accounts/export.rclone.conf", func(w http.ResponseWriter, r *http.Request) {
		unionName := r.URL.Query().Get("union_name")
		createPolicy := r.URL.Query().Get("create_policy")
		confContent := rclone.GenerateConfig(store.List(), unionName, createPolicy)

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", "attachment; filename=\"rclone.conf\"")
		_, _ = w.Write([]byte(confContent))
	})

	// Export Cloudflare Worker Script
	mux.HandleFunc("GET /api/accounts/export.worker.js", func(w http.ResponseWriter, r *http.Request) {
		scriptContent := worker.GenerateCloudflareWorker(store.List())

		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.Header().Set("Content-Disposition", "attachment; filename=\"worker.js\"")
		_, _ = w.Write([]byte(scriptContent))
	})

	// Sync File Index
	mux.HandleFunc("POST /api/accounts/sync-index", func(w http.ResponseWriter, r *http.Request) {
		addLog("info", "Starting file index synchronization...")
		go func() {
			tree, err := mega.CrawlAllAccounts(store.List(), cfg.MegatoolsPath)
			if err != nil {
				addLog("error", "Index sync failed: " + err.Error())
				return
			}
			
			// Save index.json to DATA_DIR
			data, _ := json.MarshalIndent(tree, "", "  ")
			_ = os.WriteFile(cfg.DataDir + "/index.json", data, 0644)
			addLog("success", "File index synchronized successfully.")
		}()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "sync started"})
	})

	// Fix Failed Accounts (re-test login to recover verified accounts)
	mux.HandleFunc("POST /api/accounts/fix-failed", func(w http.ResponseWriter, r *http.Request) {
		addLog("info", i18n.T("en", "msg_fixing_failed_started"))
		accounts := store.List()
		tested := 0
		recovered := 0

		for _, acc := range accounts {
			if acc.Status != "failed" {
				continue
			}
			tested++
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			err := megaClient.LoginCheck(ctx, acc.Email, acc.MegaPassword)
			cancel()

			if err == nil {
				_ = store.FixAccount(acc.Email)
				recovered++
				addLog("success", i18n.T("en", "msg_account_fixed", acc.Email))
			} else {
				addLog("warning", i18n.T("en", "msg_account_still_failed", acc.Email, err.Error()))
			}
			time.Sleep(1 * time.Second)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"tested":    tested,
			"recovered": recovered,
		})
	})

	// Recheck / Fix individual account
	mux.HandleFunc("POST /api/accounts/recheck", func(w http.ResponseWriter, r *http.Request) {
		email := r.URL.Query().Get("email")
		if email == "" {
			var body struct {
				Email string `json:"email"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			email = body.Email
		}

		acc, ok := store.Get(email)
		if !ok {
			http.Error(w, "Account not found", http.StatusNotFound)
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		err := megaClient.LoginCheck(ctx, acc.Email, acc.MegaPassword)
		cancel()

		if err == nil {
			_ = store.FixAccount(acc.Email)
			addLog("success", i18n.T("en", "msg_account_fixed", acc.Email))
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"status":  "verified",
				"message": "Account verified and recovered",
			})
		} else {
			_ = store.MarkFailed(acc.Email, err.Error())
			addLog("warning", i18n.T("en", "msg_account_still_failed", acc.Email, err.Error()))
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"status": "failed",
				"error":  err.Error(),
			})
		}
	})

	// Migration / Import accounts from other servers
	mux.HandleFunc("POST /api/accounts/import", func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read body", http.StatusBadRequest)
			return
		}

		raw := strings.TrimSpace(string(bodyBytes))
		var accountsToImport []*storage.Account

		if strings.HasPrefix(raw, "[") {
			// Parse JSON
			if err := json.Unmarshal(bodyBytes, &accountsToImport); err != nil {
				http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
				return
			}
		} else {
			// Parse CSV
			lines := strings.Split(raw, "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(strings.ToLower(line), "email") {
					continue
				}
				parts := strings.Split(line, ",")
				if len(parts) >= 2 {
					acc := &storage.Account{
						Email:        strings.Trim(strings.TrimSpace(parts[0]), "\""),
						MegaPassword: strings.Trim(strings.TrimSpace(parts[1]), "\""),
						Status:       "verified",
						CreatedAt:    time.Now(),
					}
					if len(parts) > 2 {
						acc.Usage = strings.Trim(strings.TrimSpace(parts[2]), "\"")
					}
					if len(parts) > 3 {
						acc.MailtmPassword = strings.Trim(strings.TrimSpace(parts[3]), "\"")
					}
					if len(parts) > 4 {
						acc.MailtmID = strings.Trim(strings.TrimSpace(parts[4]), "\"")
					}
					if len(parts) > 5 {
						acc.Purpose = strings.Trim(strings.TrimSpace(parts[5]), "\"")
					}
					accountsToImport = append(accountsToImport, acc)
				}
			}
		}

		imported, skipped, err := store.ImportAccounts(accountsToImport)
		if err != nil {
			http.Error(w, "Import error: "+err.Error(), http.StatusInternalServerError)
			return
		}

		addLog("info", i18n.T("en", "msg_migration_success", imported, skipped))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"imported": imported,
			"skipped":  skipped,
			"total":    len(accountsToImport),
		})
	})

	// API KeepAlive Now
	mux.HandleFunc("POST /api/accounts/keepalive", func(w http.ResponseWriter, r *http.Request) {
		triggered := sched.TriggerNow()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"triggered": triggered,
		})
	})

	// Webhook Mailgun Inbound
	mux.HandleFunc("POST /api/webhooks/mailgun", func(w http.ResponseWriter, r *http.Request) {
		payload, err := mailgun.ParseWebhook(r)
		if err != nil {
			addLog("warning", fmt.Sprintf("Failed to parse Mailgun webhook: %v", err))
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}

		recipient := strings.ToLower(strings.Trim(strings.TrimSpace(payload.Recipient), "<>\"'"))

		// Verify signature if key is set
		if cfg.MailgunSigningKey != "" {
			if !mailgun.VerifySignature(cfg.MailgunSigningKey, payload.Timestamp, payload.Token, payload.Signature) {
				addLog("error", fmt.Sprintf("Mailgun webhook signature invalid for recipient %s. Check MAILGUN_WEBHOOK_SIGNING_KEY.", recipient))
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
		}

		addLog("info", i18n.T("en", "msg_webhook_received", recipient))

		// Search account: try exact match first
		acc, exists := store.Get(recipient)

		// If account already verified, ignore follow-up onboarding/welcome emails gracefully
		if exists && acc.Status == "verified" {
			addLog("info", fmt.Sprintf("Account %s is already verified, ignoring follow-up onboarding email.", acc.Email))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Account already verified"))
			return
		}

		if !exists || acc.VerifyCommand == "" {
			// Fallback: try matching by local-part username (e.g., mega2) across pending accounts
			localPart := recipient
			if idx := strings.Index(recipient, "@"); idx != -1 {
				localPart = recipient[:idx]
			}
			for _, candidate := range store.List() {
				candLocal := candidate.Email
				if idx := strings.Index(candidate.Email, "@"); idx != -1 {
					candLocal = candidate.Email[:idx]
				}
				if strings.EqualFold(candLocal, localPart) {
					if candidate.Status == "verified" {
						addLog("info", fmt.Sprintf("Account %s is already verified, ignoring follow-up onboarding email.", candidate.Email))
						w.WriteHeader(http.StatusOK)
						_, _ = w.Write([]byte("Account already verified"))
						return
					}
					if candidate.VerifyCommand != "" {
						acc = candidate
						exists = true
						addLog("info", fmt.Sprintf("Matched pending account %s by username prefix '%s'", candidate.Email, localPart))
						break
					}
				}
			}
		}

		if !exists || acc.VerifyCommand == "" {
			addLog("warning", fmt.Sprintf("No pending registration found for recipient %s (ensure generation was started first)", recipient))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("No matching pending account"))
			return
		}

		// Extract URL: try plain body, then HTML body, then stripped text
		confirmURL, err := mega.ExtractConfirmationURL(payload.BodyPlain)
		if err != nil {
			confirmURL, err = mega.ExtractConfirmationURL(payload.BodyHtml)
		}
		if err != nil {
			confirmURL, err = mega.ExtractConfirmationURL(payload.StrippedText)
		}

		if err != nil {
			addLog("error", fmt.Sprintf("Failed to extract confirmation URL from email: %v", err))
			_ = store.MarkFailed(acc.Email, "No confirmation link in email body")
			w.WriteHeader(http.StatusOK)
			return
		}

		addLog("info", i18n.T("en", "msg_webhook_link_found", confirmURL))

		// Execute verification with megatools
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		err = megaClient.Verify(ctx, acc.VerifyCommand, confirmURL)
		if err != nil {
			addLog("error", i18n.T("en", "msg_account_failed", acc.Email, err.Error()))
			_ = store.MarkFailed(acc.Email, err.Error())
		} else {
			_ = store.MarkVerified(acc.Email)
			addLog("success", i18n.T("en", "msg_account_verified", acc.Email))
			addLog("info", i18n.T("en", "msg_csv_saved"))
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// Webhook Simulation (for testing links without public Mailgun binding)
	mux.HandleFunc("POST /api/webhooks/simulate", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Email string `json:"email"`
			Body  string `json:"body"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		confirmURL, err := mega.ExtractConfirmationURL(req.Body)
		if err != nil {
			http.Error(w, "No URL found in body", http.StatusBadRequest)
			return
		}

		// If email specified, find that account; otherwise find any pending account
		var targetAcc *storage.Account
		if req.Email != "" {
			if a, ok := store.Get(req.Email); ok {
				targetAcc = a
			}
		} else {
			for _, a := range store.List() {
				if a.Status == "pending" && a.VerifyCommand != "" {
					targetAcc = a
					break
				}
			}
		}

		if targetAcc == nil {
			http.Error(w, "No pending account with verify command found to match link", http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		err = megaClient.Verify(ctx, targetAcc.VerifyCommand, confirmURL)
		if err != nil {
			_ = store.MarkFailed(targetAcc.Email, err.Error())
			addLog("error", fmt.Sprintf("Verification failed: %v", err))
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		_ = store.MarkVerified(targetAcc.Email)
		addLog("success", i18n.T("en", "msg_account_verified", targetAcc.Email))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  "verified",
			"email":   targetAcc.Email,
			"link":    confirmURL,
			"message": "Account successfully verified",
		})
	})

	server := &http.Server{
		Addr:         "0.0.0.0:" + cfg.Port,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}
