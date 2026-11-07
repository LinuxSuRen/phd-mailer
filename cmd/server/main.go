// phd-mailer：读取 data/emails.yaml，通过腾讯邮箱（QQ 邮箱）SMTP 发送套磁邮件。
// 默认 dry-run 模式，不会真实发信；加 -dry-run=false 才会真正发送。
package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/linuxsuren/phd-mailer/internal/ledger"
	"github.com/linuxsuren/phd-mailer/internal/server"
)

func main() {
	var (
		addr    = flag.String("addr", ":8080", "HTTP 监听地址")
		data    = flag.String("data", "data/emails.yaml", "邮件数据文件路径")
		prompt  = flag.String("prompt", "docs/ai-prompt.md", "AI 提示词文件路径（控制台可查看与修改）")
		ledgerP = flag.String("ledger", "data/sent-ledger.json", "发送台账文件路径（去重依据）")
		dryRun  = flag.Bool("dry-run", true, "试运行模式：默认开启，仅演练不真实发信")
	)
	flag.Parse()

	l, err := ledger.New(*ledgerP)
	if err != nil {
		log.Fatalf("加载发送台账失败：%v", err)
	}
	s, err := server.New(*data, *prompt, l, *dryRun)
	if err != nil {
		log.Fatalf("加载数据文件失败：%v", err)
	}

	mux := http.NewServeMux()
	s.Routes(mux)
	serveWeb(mux)

	mode := "dry-run（不会真实发信）"
	if !*dryRun {
		mode = "真实发送"
	}
	log.Printf("phd-mailer 已启动：http://localhost%s 模式：%s 台账：%s", *addr, mode, *ledgerP)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
	}
}

// serveWeb 托管前端构建产物 web/dist（存在时），便于单二进制部署。
func serveWeb(mux *http.ServeMux) {
	dist := filepath.Join("web", "dist")
	if info, err := os.Stat(dist); err != nil || !info.IsDir() {
		return
	}
	fs := http.FileServer(http.Dir(dist))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.HasPrefix(path, "/api/") {
			http.NotFound(w, r)
			return
		}
		if _, err := os.Stat(filepath.Join(dist, path)); err != nil {
			// 前端为单页应用，未知路径回退到 index.html
			http.ServeFile(w, r, filepath.Join(dist, "index.html"))
			return
		}
		fs.ServeHTTP(w, r)
	})
}
