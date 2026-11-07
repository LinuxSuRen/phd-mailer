// Package mailer 通过腾讯邮箱（QQ 邮箱）的 SMTP 服务发送纯文本邮件。
// 使用 465 端口的隐式 TLS 连接，登录凭证为 QQ 邮箱授权码（非登录密码）。
package mailer

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net/smtp"
	"strings"

	"net/textproto"
	"mime"
)

// Config SMTP 连接与发件人配置。
type Config struct {
	Host       string // 例如 smtp.qq.com
	Port       int    // 例如 465
	User       string // 发件邮箱地址，例如 xxx@qq.com
	AuthCode   string // QQ 邮箱 SMTP 授权码
	SenderName string // 发件人显示名
}

// Validate 校验配置完整性。
func (c Config) Validate() error {
	if c.Host == "" || c.Port == 0 {
		return errors.New("SMTP 主机或端口未配置")
	}
	if c.User == "" || c.AuthCode == "" {
		return errors.New("SMTP 账号或授权码未配置")
	}
	return nil
}

// Send 发送一封纯文本邮件（UTF-8）。body 为正文原文，函数内部不做签名拼接。
func Send(cfg Config, to, subject, body string) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	if !strings.Contains(to, "@") {
		return fmt.Errorf("收件地址无效：%s", to)
	}

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: cfg.Host})
	if err != nil {
		return fmt.Errorf("连接 SMTP 失败：%w", err)
	}
	client, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		return fmt.Errorf("建立 SMTP 会话失败：%w", err)
	}
	defer client.Close()

	auth := smtp.PlainAuth("", cfg.User, cfg.AuthCode, cfg.Host)
	if err = client.Auth(auth); err != nil {
		return fmt.Errorf("SMTP 认证失败（请检查授权码）：%w", err)
	}
	if err = client.Mail(cfg.User); err != nil {
		return fmt.Errorf("设置发件人失败：%w", err)
	}
	if err = client.Rcpt(to); err != nil {
		return fmt.Errorf("设置收件人失败：%w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("打开数据通道失败：%w", err)
	}
	if _, err = w.Write(buildMessage(cfg, to, subject, body)); err != nil {
		return fmt.Errorf("写入邮件内容失败：%w", err)
	}
	if err = w.Close(); err != nil {
		return fmt.Errorf("提交邮件失败：%w", err)
	}
	return client.Quit()
}

func buildMessage(cfg Config, to, subject, body string) []byte {
	enc := mime.QEncoding.Encode("UTF-8", cfg.SenderName)
	return []byte(strings.Join([]string{
		"From: " + enc + " <" + cfg.User + ">",
		"To: <" + to + ">",
		"Subject: " + textproto.TrimString(mime.QEncoding.Encode("UTF-8", subject)),
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"Content-Transfer-Encoding: 8bit",
		"",
		body,
	}, "\r\n"))
}
