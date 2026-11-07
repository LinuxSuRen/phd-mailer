package ledger

import (
	"path/filepath"
	"testing"
	"time"
)

func TestLedgerDedup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.json")
	l, err := New(path)
	if err != nil {
		t.Fatalf("新建台账失败：%v", err)
	}
	if l.Sent("id-1", "a@x.com") {
		t.Fatal("空台账不应命中")
	}
	if err := l.Add(Record{ContactID: "id-1", Email: "a@x.com", SentAt: time.Now()}); err != nil {
		t.Fatalf("写入台账失败：%v", err)
	}
	if !l.Sent("id-1", "other@x.com") {
		t.Fatal("同一 contact id 换邮箱仍应命中去重")
	}
	if !l.Sent("id-2", "a@x.com") {
		t.Fatal("同一邮箱换 contact id 仍应命中去重")
	}
	if l.Sent("id-2", "b@x.com") {
		t.Fatal("无关联系人不应命中")
	}

	// 持久化后重载，去重仍生效
	l2, err := New(path)
	if err != nil {
		t.Fatalf("重载台账失败：%v", err)
	}
	if !l2.Sent("id-1", "a@x.com") {
		t.Fatal("重载后已发送记录应命中去重")
	}
}
