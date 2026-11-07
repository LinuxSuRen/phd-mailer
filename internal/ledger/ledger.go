// Package ledger 维护已发送台账，保证同一联系人（按 id 与收件邮箱双重判断）
// 永远只收到一封邮件。台账以 JSON 文件持久化，崩溃安全（写临时文件后原子改名）。
package ledger

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Record 一条成功发送记录。
type Record struct {
	ContactID string    `json:"contact_id"`
	Email     string    `json:"email"`
	Subject   string    `json:"subject"`
	SentAt    time.Time `json:"sent_at"`
}

// Ledger 并发安全的发送台账。
type Ledger struct {
	mu      sync.RWMutex
	path    string
	byID    map[string]Record
	byEmail map[string]Record
}

// New 创建并加载台账文件（不存在则视为空台账）。
func New(path string) (*Ledger, error) {
	l := &Ledger{
		path:    path,
		byID:    map[string]Record{},
		byEmail: map[string]Record{},
	}
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return l, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取台账失败：%w", err)
	}
	var records []Record
	if err := json.Unmarshal(raw, &records); err != nil {
		return nil, fmt.Errorf("解析台账失败：%w", err)
	}
	for _, r := range records {
		l.byID[r.ContactID] = r
		l.byEmail[r.Email] = r
	}
	return l, nil
}

// Sent 按 id 或邮箱任一命中即视为已发送过。
func (l *Ledger) Sent(contactID, email string) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	_, okID := l.byID[contactID]
	_, okEmail := l.byEmail[email]
	return okID || okEmail
}

// Lookup 查询发送记录：按 id 或邮箱任一命中即返回。
func (l *Ledger) Lookup(contactID, email string) (Record, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if r, ok := l.byID[contactID]; ok {
		return r, true
	}
	r, ok := l.byEmail[email]
	return r, ok
}

// Add 记录一次成功发送并持久化。
func (l *Ledger) Add(r Record) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.byID[r.ContactID] = r
	l.byEmail[r.Email] = r
	return l.saveLocked()
}

func (l *Ledger) saveLocked() error {
	records := make([]Record, 0, len(l.byID))
	for _, r := range l.byID {
		records = append(records, r)
	}
	raw, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		return err
	}
	tmp := l.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, l.path)
}
