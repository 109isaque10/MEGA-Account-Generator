package storage

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Account struct {
	Email           string     `json:"email"`
	MegaPassword    string     `json:"mega_password"`
	Usage           string     `json:"usage"`
	MailtmPassword  string     `json:"mailtm_password"`
	MailtmID        string     `json:"mailtm_id"`
	Purpose         string     `json:"purpose"`
	Status          string     `json:"status"` // "verified", "pending", "failed"
	CreatedAt       time.Time  `json:"created_at"`
	LastLoginAt     *time.Time `json:"last_login_at,omitempty"`
	LastLoginStatus string     `json:"last_login_status,omitempty"`
	VerifyCommand   string     `json:"verify_command,omitempty"`
	ErrorMessage    string     `json:"error_message,omitempty"`
}

type Store struct {
	mu       sync.RWMutex
	accounts map[string]*Account
	csvPath  string
	jsonPath string
}

func New(csvPath, jsonPath string) (*Store, error) {
	s := &Store{
		accounts: make(map[string]*Account),
		csvPath:  csvPath,
		jsonPath: jsonPath,
	}

	// Ensure directory exists
	if dir := filepath.Dir(csvPath); dir != "" && dir != "." {
		_ = os.MkdirAll(dir, 0755)
	}
	if dir := filepath.Dir(jsonPath); dir != "" && dir != "." {
		_ = os.MkdirAll(dir, 0755)
	}

	// 1. Try loading from json first if available (contains richer metadata)
	_ = s.loadFromJSON()

	// 2. Synchronize with accounts.csv (creates file with headers if missing, imports any rows missing in memory)
	if err := s.syncWithCSV(); err != nil {
		return nil, fmt.Errorf("failed to sync with CSV: %w", err)
	}

	return s, nil
}

func (s *Store) syncWithCSV() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if CSV exists
	file, err := os.Open(s.csvPath)
	if os.IsNotExist(err) {
		// Create file with exact legacy headers
		wFile, err := os.OpenFile(s.csvPath, os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		defer wFile.Close()
		w := csv.NewWriter(wFile)
		_ = w.Write([]string{"Email", "MEGA Password", "Usage", "Mail.tm Password", "Mail.tm ID", "Purpose"})
		w.Flush()
		return nil
	} else if err != nil {
		return err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	// Read header
	header, err := reader.Read()
	if err != nil && err != io.EOF {
		return err
	}
	_ = header

	// Read rows
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil || len(record) < 2 {
			continue
		}
		email := strings.TrimSpace(record[0])
		password := strings.TrimSpace(record[1])
		if email == "" || email == "Email" {
			continue
		}

		usage := "-"
		mailPass := "-"
		mailID := "-"
		purpose := "-"
		if len(record) > 2 {
			usage = record[2]
		}
		if len(record) > 3 {
			mailPass = record[3]
		}
		if len(record) > 4 {
			mailID = record[4]
		}
		if len(record) > 5 {
			purpose = record[5]
		}

		if _, exists := s.accounts[email]; !exists {
			s.accounts[email] = &Account{
				Email:          email,
				MegaPassword:   password,
				Usage:          usage,
				MailtmPassword: mailPass,
				MailtmID:       mailID,
				Purpose:        purpose,
				Status:         "verified",
				CreatedAt:      time.Now(),
			}
		}
	}

	return nil
}

func (s *Store) loadFromJSON() error {
	data, err := os.ReadFile(s.jsonPath)
	if err != nil {
		return err
	}
	var list []*Account
	if err := json.Unmarshal(data, &list); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, acc := range list {
		if acc != nil && acc.Email != "" {
			s.accounts[acc.Email] = acc
		}
	}
	return nil
}

func (s *Store) saveToJSON() error {
	list := make([]*Account, 0, len(s.accounts))
	for _, acc := range s.accounts {
		list = append(list, acc)
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.jsonPath, data, 0644)
}

func (s *Store) appendToCSV(acc *Account) error {
	file, err := os.OpenFile(s.csvPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	w := csv.NewWriter(file)
	err = w.Write([]string{
		acc.Email,
		acc.MegaPassword,
		acc.Usage,
		acc.MailtmPassword,
		acc.MailtmID,
		acc.Purpose,
	})
	if err != nil {
		return err
	}
	w.Flush()
	return w.Error()
}

func (s *Store) SavePending(acc *Account) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	acc.Status = "pending"
	if acc.CreatedAt.IsZero() {
		acc.CreatedAt = time.Now()
	}
	if acc.Usage == "" {
		acc.Usage = "-"
	}
	if acc.MailtmPassword == "" {
		acc.MailtmPassword = "-"
	}
	if acc.MailtmID == "" {
		acc.MailtmID = "-"
	}
	if acc.Purpose == "" {
		acc.Purpose = "-"
	}

	s.accounts[acc.Email] = acc
	return s.saveToJSON()
}

func (s *Store) MarkVerified(email string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	acc, exists := s.accounts[email]
	if !exists {
		return fmt.Errorf("account %s not found", email)
	}

	acc.Status = "verified"
	acc.ErrorMessage = ""
	_ = s.saveToJSON()
	return s.appendToCSV(acc)
}

func (s *Store) MarkFailed(email, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	acc, exists := s.accounts[email]
	if !exists {
		return fmt.Errorf("account %s not found", email)
	}

	acc.Status = "failed"
	acc.ErrorMessage = reason
	return s.saveToJSON()
}

func (s *Store) RecordLogin(email string, success bool, errMsg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	acc, exists := s.accounts[email]
	if !exists {
		return fmt.Errorf("account %s not found", email)
	}

	now := time.Now()
	acc.LastLoginAt = &now
	if success {
		acc.LastLoginStatus = "success"
		acc.ErrorMessage = ""
	} else {
		acc.LastLoginStatus = "failed"
		acc.ErrorMessage = errMsg
	}
	return s.saveToJSON()
}

func (s *Store) Get(email string) (*Account, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	acc, exists := s.accounts[email]
	if !exists {
		return nil, false
	}
	copied := *acc
	return &copied, true
}

func (s *Store) List() []*Account {
	s.mu.RLock()
	defer s.mu.RUnlock()

	res := make([]*Account, 0, len(s.accounts))
	for _, acc := range s.accounts {
		copied := *acc
		res = append(res, &copied)
	}
	return res
}

func (s *Store) Count() (total, active, pending, failed int) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, acc := range s.accounts {
		total++
		switch acc.Status {
		case "verified":
			active++
		case "pending":
			pending++
		case "failed":
			failed++
		}
	}
	return
}

func (s *Store) GetNextSequentialIndex(pattern string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	maxIndex := 0
	for email := range s.accounts {
		var idx int
		if _, err := fmt.Sscanf(email, pattern, &idx); err == nil {
			if idx > maxIndex {
				maxIndex = idx
			}
		}
	}
	return maxIndex + 1
}
