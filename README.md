# phd-mailer

<p align="center">
  <a href="https://github.com/LinuxSuRen/phd-mailer">
    <img src="assets/logo.svg" width="128" alt="phd-mailer logo" />
  </a>
</p>

<p align="center">
  <a href="https://github.com/LinuxSuRen/phd-mailer">
    <img src="https://img.shields.io/badge/GitHub-LinuxSuRen%2Fphd--mailer-181717?logo=github&style=flat-square" alt="GitHub repo" />
  </a>
  <img src="https://img.shields.io/badge/license-MIT-blue?style=flat-square" alt="license MIT" />
</p>

博士申请套磁邮件发送台：用一份 YAML 文件管理所有导师联系人与邮件内容，通过腾讯邮箱（QQ 邮箱）SMTP 自动发送，**保证同一位导师永远不会收到第二封**。

适合需要向多位导师逐一发送定制邮件、又担心手滑重复发送的申请者。

## 功能特性

- **数据驱动**：所有导师信息、邮件主题与正文都维护在 [`data/emails.yaml`](data/emails.yaml) 中，改文件即改内容，发件人签名自动拼接；
- **严格去重**：发送台账按联系人 ID 与收件邮箱**双重判断**，任一命中即拒绝再次发送，台账以 JSON 文件持久化，重启程序也不失效；
- **安全默认**：程序以 dry-run 演练模式启动，**不会真实发信**，确认无误后加 `-dry-run=false` 才会真正发送；
- **批量限速**：批量发送按优先级排序依次发出，相邻间隔可配置（默认 60 秒），避免触发邮箱服务商的频率限制；
- **Web 控制台**：Vue 3 管理台，支持筛选、查看每封邮件全文、勾选批量发送、在线配置 SMTP；
- **AI 提示词**：内置 [docs/ai-prompt.md](docs/ai-prompt.md)，控制台中可一键复制给任意 AI 助手，批量生成符合本项目 YAML 格式的套磁邮件，提示词本身可在线修改。

## 快速开始

### 环境要求

- Go 1.24+
- Node.js 20+（仅构建前端时需要）

### 安装

从 [GitHub Releases](https://github.com/LinuxSuRen/phd-mailer/releases) 下载对应平台的二进制（支持 linux/darwin/windows 的 amd64 与 arm64），连同 `phd-mailer-web-dist.zip` 解压出的 `web/dist` 目录放到同一工作目录，即可运行。

### 构建与启动

```bash
# 构建前端（已在 web/dist 产出，可跳过）
cd web && npm install && npm run build && cd ..

# 启动（默认 dry-run 演练模式，不会真实发信）
go run ./cmd/server

# 打开控制台
open http://localhost:8080
```

### 配置腾讯邮箱

1. 登录 [QQ 邮箱网页版](https://mail.qq.com/) → 设置 → 账号 → 开启 SMTP 服务，按提示生成**授权码**（不是 QQ 密码）；
2. 在控制台右上角「SMTP 配置」中填入邮箱地址与授权码；也可以通过环境变量提供：

```bash
export PHD_MAILER_SMTP_USER=someone@qq.com
export PHD_MAILER_SMTP_AUTH_CODE=你的授权码
```

### 真实发送

演练确认无误后，关闭演练模式重启：

```bash
go run ./cmd/server -dry-run=false
```

在控制台勾选导师，点击「发送选中的」即可。程序会自动跳过台账中已发送过的导师。

### 常用参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-addr` | `:8080` | HTTP 监听地址 |
| `-data` | `data/emails.yaml` | 邮件数据文件路径 |
| `-ledger` | `data/sent-ledger.json` | 发送台账路径（去重依据，**请勿删除**） |
| `-dry-run` | `true` | 演练模式开关 |

## 数据文件格式

```yaml
version: 1
sender:
  name: 张三
  phone_wechat: "13800000000"
smtp:
  host: smtp.qq.com
  port: 465
  user: ""          # 也可用环境变量 PHD_MAILER_SMTP_USER
  auth_code: ""     # 也可用环境变量 PHD_MAILER_SMTP_AUTH_CODE
defaults:
  interval_seconds: 60
contacts:
  - id: buaa-example        # 全局唯一，去重主键之一
    name: 某老师
    university: 某大学
    department: 某学院
    email: someone@example.edu
    priority: 1             # 越小越先发送
    tags: [标签]
    subject: 邮件主题
    body: |
      邮件正文，发送时自动在末尾拼接 sender 签名。
```

## 用 AI 批量生成邮件

项目内置一份提示词（[docs/ai-prompt.md](docs/ai-prompt.md)）：拷贝给任意 AI 助手、替换其中的个人背景与目标导师信息，即可按本项目的 YAML 格式批量生成套磁邮件。控制台右上角「AI 提示词」支持在线编辑与复制全文。

生成的 YAML 不需要手改文件：控制台「导入数据」支持**粘贴内容或选择文件**导入，兼容完整数据文件与 AI 输出的 `contacts` 列表两种格式，SMTP 配置与签名自动保留；「导出」可随时下载当前完整数据备份。

## 项目结构

```text
assets/              # 项目 logo
cmd/server/          # 程序入口
docs/ai-prompt.md    # AI 提示词（可在控制台编辑）
internal/mailer/     # SMTP 发送（QQ 邮箱 465 隐式 TLS）
internal/ledger/     # 发送台账（去重 + 持久化）
internal/server/     # REST API 与配置读写
web/                 # Vue 3 + Vite + Element Plus 控制台
data/emails.yaml     # 导师与邮件数据（含隐私，不入库，见 emails.example.yaml）
```

## 注意事项

- 邮件为纯文本格式，暂不支持附件。正文中提到「附简历」时，建议导师回复后再补发简历，或在正文末尾附上个人主页链接；
- 台账文件 `sent-ledger.json` 是防重复发送的唯一依据，请务必备份；
- 请合理配置发送间隔，控制单日发送量，避免邮箱账号被服务商限流。

## 贡献

欢迎通过 issue 与 PR 参与：一个 PR 只包含一类修改，禁止直接向 main 分支提交。

## License

[MIT](LICENSE)
