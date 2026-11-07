// Package server 提供 REST API：联系人列表、SMTP 配置读写、批量发送（限速+去重）。
package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/linuxsuren/phd-mailer/internal/ledger"
	"github.com/linuxsuren/phd-mailer/internal/mailer"
)

// Contact 一位导师及其套磁邮件内容。
type Contact struct {
	ID         string   `yaml:"id" json:"id"`
	Name       string   `yaml:"name" json:"name"`
	University string   `yaml:"university" json:"university"`
	Department string   `yaml:"department" json:"department"`
	Email      string   `yaml:"email" json:"email"`
	Priority   int      `yaml:"priority" json:"priority"`
	Tags       []string `yaml:"tags" json:"tags"`
	Subject    string   `yaml:"subject" json:"subject"`
	Body       string   `yaml:"body" json:"body"`
}

// Sender 发件人信息，用于自动拼接签名。
type Sender struct {
	Name        string `yaml:"name" json:"name"`
	PhoneWechat string `yaml:"phone_wechat" json:"phone_wechat"`
	TitleLine   string `yaml:"title_line" json:"title_line"`
}

// DataFile data/emails.yaml 的完整结构。
type DataFile struct {
	Version  int `yaml:"version" json:"version"`
	Sender   Sender `yaml:"sender" json:"sender"`
	SMTP     mailer.Config `yaml:"smtp" json:"smtp"`
	Defaults struct {
		IntervalSeconds int `yaml:"interval_seconds" json:"interval_seconds"`
	} `yaml:"defaults" json:"defaults"`
	Contacts []Contact `yaml:"contacts" json:"contacts"`
}

// SendResult 单个联系人的处理结果。
type SendResult struct {
	ContactID string `json:"contact_id"`
	Name      string `json:"name"`
	Status    string `json:"status"` // sent / dry_run / skipped_duplicate / failed
	Detail    string `json:"detail,omitempty"`
}

// Server 应用状态。
type Server struct {
	mu        sync.Mutex
	dataPath  string
	promptPath string
	data      *DataFile
	ledger    *ledger.Ledger
	dryRun    bool
}

// New 加载数据文件并构建服务。promptPath 为 AI 提示词文件路径。
func New(dataPath, promptPath string, l *ledger.Ledger, dryRun bool) (*Server, error) {
	raw, err := os.ReadFile(dataPath)
	if err != nil {
		return nil, fmt.Errorf("读取数据文件失败：%w", err)
	}
	var d DataFile
	if err := yaml.Unmarshal(raw, &d); err != nil {
		return nil, fmt.Errorf("解析数据文件失败：%w", err)
	}
	if len(d.Contacts) == 0 {
		return nil, fmt.Errorf("数据文件中没有联系人")
	}
	d.SMTP.User = firstNonEmpty(d.SMTP.User, os.Getenv("PHD_MAILER_SMTP_USER"))
	d.SMTP.AuthCode = firstNonEmpty(d.SMTP.AuthCode, os.Getenv("PHD_MAILER_SMTP_AUTH_CODE"))
	return &Server{dataPath: dataPath, promptPath: promptPath, data: &d, ledger: l, dryRun: dryRun}, nil
}

// Routes 注册全部 API（前缀 /api）。
func (s *Server) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/contacts", s.handleContacts)
	mux.HandleFunc("GET /api/config", s.handleGetConfig)
	mux.HandleFunc("PUT /api/config", s.handlePutConfig)
	mux.HandleFunc("POST /api/send", s.handleSend)
	mux.HandleFunc("POST /api/contacts/import", s.handleImportContacts)
	mux.HandleFunc("GET /api/data/export", s.handleExportData)
	mux.HandleFunc("GET /api/prompt", s.handleGetPrompt)
	mux.HandleFunc("PUT /api/prompt", s.handlePutPrompt)
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	})
}

func (s *Server) handleContacts(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	type row struct {
		Contact
		Sent    bool      `json:"sent"`
		SentAt  time.Time `json:"sent_at,omitempty"`
	}
	rows := make([]row, 0, len(s.data.Contacts))
	for _, c := range s.data.Contacts {
		r := row{Contact: c}
		if rec, ok := s.ledger.Lookup(c.ID, c.Email); ok {
			r.Sent = true
			r.SentAt = rec.SentAt
		}
		rows = append(rows, r)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Priority != rows[j].Priority {
			return rows[i].Priority < rows[j].Priority
		}
		return rows[i].ID < rows[j].ID
	})
	writeJSON(w, http.StatusOK, map[string]any{"data": rows})
}

func (s *Server) handleGetConfig(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"smtp_host":       s.data.SMTP.Host,
			"smtp_port":       s.data.SMTP.Port,
			"smtp_user":       s.data.SMTP.User,
			"smtp_configured": s.data.SMTP.AuthCode != "",
			"sender_name":     s.data.SMTP.SenderName,
			"interval_seconds": s.data.Defaults.IntervalSeconds,
			"dry_run":         s.dryRun,
		},
	})
}

// configUpdate 前端可修改的配置子集。
type configUpdate struct {
	SMTPUser        string `json:"smtp_user"`
	SMTPAuthCode    string `json:"smtp_auth_code"`
	IntervalSeconds int    `json:"interval_seconds"`
}

func (s *Server) handlePutConfig(w http.ResponseWriter, r *http.Request) {
	var in configUpdate
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "配置格式不正确")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(in.SMTPUser) != "" {
		s.data.SMTP.User = strings.TrimSpace(in.SMTPUser)
	}
	if strings.TrimSpace(in.SMTPAuthCode) != "" {
		s.data.SMTP.AuthCode = strings.TrimSpace(in.SMTPAuthCode)
	}
	if in.IntervalSeconds >= 0 {
		s.data.Defaults.IntervalSeconds = in.IntervalSeconds
	}
	if err := s.persistLocked(); err != nil {
		writeErr(w, http.StatusInternalServerError, "配置保存失败，请检查文件权限")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": "ok"})
}

// sendRequest 批量发送请求。
type sendRequest struct {
	IDs    []string `json:"ids"`
	DryRun *bool    `json:"dry_run,omitempty"`
}

func (s *Server) handleSend(w http.ResponseWriter, r *http.Request) {
	var req sendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "请求格式不正确")
		return
	}
	dryRun := s.dryRun
	if req.DryRun != nil {
		dryRun = *req.DryRun
	}

	s.mu.Lock()
	byID := map[string]Contact{}
	for _, c := range s.data.Contacts {
		byID[c.ID] = c
	}
	sender := s.data.Sender
	smtpCfg := s.data.SMTP
	interval := s.data.Defaults.IntervalSeconds
	s.mu.Unlock()

	results := make([]SendResult, 0, len(req.IDs))
	// 按数据文件中的 priority 排序发送，重要的人先发
	ordered := s.orderByID(byID, req.IDs)
	realSent := 0
	for _, id := range ordered {
		c := byID[id]
		res := SendResult{ContactID: id, Name: c.Name}
		switch {
		case s.ledger.Sent(c.ID, c.Email):
			res.Status = "skipped_duplicate"
			res.Detail = "台账中已有该联系人或该邮箱的发送记录，已跳过以防重复"
		case dryRun:
			res.Status = "dry_run"
		default:
			body := c.Body + "\n\n" + sender.Name + "\n电话/微信：" + sender.PhoneWechat + "\n"
			if err := mailer.Send(smtpCfg, c.Email, c.Subject, body); err != nil {
				res.Status = "failed"
				res.Detail = err.Error()
			} else {
				_ = s.ledger.Add(ledger.Record{
					ContactID: c.ID, Email: c.Email, Subject: c.Subject, SentAt: time.Now(),
				})
				res.Status = "sent"
				realSent++
			}
		}
		results = append(results, res)
		// 限速：仅在真实发送之间等待
		if realSent > 0 && interval > 0 && realSent < len(ordered) {
			time.Sleep(time.Duration(interval) * time.Second)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": results})
}

func (s *Server) orderByID(byID map[string]Contact, ids []string) []string {
	prio := map[string]int{}
	for _, c := range byID {
		prio[c.ID] = c.Priority
	}
	ordered := append([]string(nil), ids...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return prio[ordered[i]] < prio[ordered[j]]
	})
	return ordered
}

// persistLocked 将配置写回数据文件（调用方需持锁）。
func (s *Server) persistLocked() error {
	raw, err := yaml.Marshal(s.data)
	if err != nil {
		return err
	}
	return os.WriteFile(s.dataPath, raw, 0o600)
}

// importRequest 导入请求：content 为用户粘贴或从文件读取的 YAML 文本。
type importRequest struct {
	Content string `json:"content"`
}

// handleImportContacts 支持两种导入内容：
//  1. 完整数据文件（含 version/sender/smtp/contacts），整体替换；
//  2. 仅 contacts 列表（AI 生成物的典型输出），替换联系人但保留现有 sender/smtp/defaults。
func (s *Server) handleImportContacts(w http.ResponseWriter, r *http.Request) {
	var req importRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "请求格式不正确")
		return
	}
	if strings.TrimSpace(req.Content) == "" {
		writeErr(w, http.StatusBadRequest, "导入内容为空")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var full DataFile
	var contacts []Contact
	if err := yaml.Unmarshal([]byte(req.Content), &full); err == nil && len(full.Contacts) > 0 {
		contacts = full.Contacts
		// 完整文件导入：SMTP 凭证为空时保留现有配置，避免导入覆盖掉已配置的授权码
		if full.SMTP.User == "" || full.SMTP.AuthCode == "" {
			full.SMTP = s.data.SMTP
		}
	} else if err := yaml.Unmarshal([]byte(req.Content), &struct {
		Contacts *[]Contact `yaml:"contacts"`
	}{Contacts: &contacts}); err != nil || len(contacts) == 0 {
		writeErr(w, http.StatusBadRequest, "无法识别导入内容：需要完整数据文件或 contacts 列表")
		return
	}

	// 基础校验：id 唯一且邮箱完整
	seen := map[string]bool{}
	for _, c := range contacts {
		if c.ID == "" || c.Email == "" || c.Subject == "" || c.Body == "" {
			writeErr(w, http.StatusBadRequest, "联系人字段不完整（需要 id、email、subject、body）")
			return
		}
		if seen[c.ID] {
			writeErr(w, http.StatusBadRequest, "联系人 id 重复："+c.ID)
			return
		}
		seen[c.ID] = true
	}

	if len(full.Contacts) > 0 {
		full.Defaults = s.data.Defaults
		if full.Sender.Name == "" {
			full.Sender = s.data.Sender
		}
		s.data = &full
	} else {
		s.data.Contacts = contacts
	}
	if err := s.persistLocked(); err != nil {
		writeErr(w, http.StatusInternalServerError, "写入数据文件失败，请检查文件权限")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]int{"contacts": len(s.data.Contacts)}})
}

// handleExportData 导出当前完整数据文件内容，便于用户备份或迁移。
func (s *Server) handleExportData(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, err := yaml.Marshal(s.data)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "导出失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"content": string(raw)}})
}

// handleGetPrompt 返回 AI 提示词文件内容（文件不存在时返回空内容）。
func (s *Server) handleGetPrompt(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	content := ""
	if raw, err := os.ReadFile(s.promptPath); err == nil {
		content = string(raw)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"content": content}})
}

// handlePutPrompt 保存用户修改后的 AI 提示词。
func (s *Server) handlePutPrompt(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "提示词格式不正确")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.promptPath), 0o755); err != nil {
		writeErr(w, http.StatusInternalServerError, "保存提示词失败，请检查文件权限")
		return
	}
	if err := os.WriteFile(s.promptPath, []byte(in.Content), 0o644); err != nil {
		writeErr(w, http.StatusInternalServerError, "保存提示词失败，请检查文件权限")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": "ok"})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"error": map[string]string{"message": msg, "code": fmt.Sprintf("HTTP%d", code)}})
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
