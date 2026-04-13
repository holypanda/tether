# Windows ↔ VPS 挂载 GUI 工具 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 构建一个单文件 Windows `.exe` GUI 工具 (`tether.exe`)，点击后能在 VPS 上建立反向 SSH 隧道 + 嵌入式 SFTP 回连，实现 VPS 上的 `~/local-code` 直接读写 Windows 本地项目文件；并提供"启动 Claude Code"按钮一键进入远程开发。

**Architecture:** 单 Go 二进制 + Fyne GUI。内嵌一个 SFTP Server（只监听 localhost），通过 `ssh -R` 反向暴露给 VPS，VPS 上的 `sshfs` 通过反向隧道回连。所有 SSH 操作用 Go `crypto/ssh` 库原生完成，Windows 侧零依赖（不需要 OpenSSH Server、不需要管理员）。

**Tech Stack:**
- Go 1.22+ (主语言)
- Fyne v2 (GUI)
- `golang.org/x/crypto/ssh` (SSH 客户端 + 服务端)
- `github.com/pkg/sftp` (SFTP 服务端)
- mingw-w64 (交叉编译到 Windows)

**项目根目录:** `/root/windows-auto-stimulator/app/`

**规范文档:** `docs/superpowers/specs/2026-04-11-windows-vps-mount-gui-design.md`

---

## 文件结构总览

```
/root/windows-auto-stimulator/app/
├── go.mod
├── go.sum
├── main.go                     # 入口 + Fyne GUI 组装
├── config/
│   ├── config.go               # Config 结构体 + JSON 读写
│   └── config_test.go
├── identity/
│   ├── identity.go             # Ed25519 密钥生成/加载
│   └── identity_test.go
├── sftpserver/
│   ├── server.go               # 嵌入式 SFTP Server
│   └── server_test.go
├── sshclient/
│   ├── client.go               # admin SSH 连接 + 反向转发
│   ├── remote.go               # 远程命令 (bootstrap/mount/unmount)
│   └── client_test.go
├── claude/
│   └── launcher.go             # 启动 Claude PowerShell 子进程
└── ui/
    ├── app.go                  # Fyne app 主入口
    ├── form.go                 # 配置表单组件
    ├── status.go               # 状态面板组件
    └── log.go                  # 日志显示组件
```

每个 package 职责单一：`config` 只管配置文件；`identity` 只管密钥；`sftpserver` 只管 SFTP 协议层；`sshclient` 只管 SSH 连接与远程命令；`claude` 只管启动 PowerShell；`ui` 只管 Fyne 组件组装；`main.go` 把所有东西连起来。

---

## 任务总览

| # | 任务 | 主要产出 |
|---|---|---|
| 0 | 环境准备：Go + mingw 工具链 | VPS 上可 `go build` + 交叉编译 |
| 1 | 项目脚手架 + git 初始化 | `go.mod` 可构建空 `main.go` |
| 2 | Config 包 | `config.Load/Save`，带测试 |
| 3 | Identity 包 | `identity.GenerateEd25519`，带测试 |
| 4 | SFTP Server 基础 | localhost SFTP server，带集成测试 |
| 5 | SFTP Server 目录根限制 | 锁定到 sharePath |
| 6 | SSH client - 密码/密钥拨号 | `sshclient.Dial`，带集成测试 |
| 7 | SSH client - 远程命令封装 | `Run` / `RunScript` |
| 8 | SSH client - 反向端口转发 | `ReverseForward` |
| 9 | Remote - bootstrap 脚本 | 首次装 sshfs + 注册 admin 公钥 |
| 10 | Remote - mount/unmount | 写临时私钥 + sshfs 命令 |
| 11 | Claude 启动器 | spawn PowerShell 子进程 |
| 12 | UI - Fyne 骨架 + 窗口布局 | 空 GUI 能运行 |
| 13 | UI - 配置表单组件 | 绑定 Config 字段 |
| 14 | UI - 状态面板 + 日志组件 | 指示灯 + 滚动日志 |
| 15 | UI - 连接按钮接线 | 点"连接"走完整 bootstrap/挂载流程 |
| 16 | UI - 启动 Claude 按钮接线 | 新 PowerShell 跑 claude |
| 17 | UI - 断开 & 关闭清理 | fusermount -u + 删临时私钥 |
| 18 | 交叉编译 Windows .exe | `tether.exe` 产物 |
| 19 | 端到端冒烟测试 | Windows 上手动验证全流程 |

---

## Task 0: 环境准备

**Files:** 无代码修改，仅安装系统依赖。

- [ ] **Step 1: 安装 Go**

```bash
cd /tmp
curl -LO https://go.dev/dl/go1.22.5.linux-amd64.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.22.5.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
export PATH=$PATH:/usr/local/go/bin
go version
```

Expected: `go version go1.22.5 linux/amd64`

- [ ] **Step 2: 安装 mingw-w64 交叉编译器**

```bash
apt-get update
apt-get install -y gcc-mingw-w64-x86-64 pkg-config libgl1-mesa-dev xorg-dev
x86_64-w64-mingw32-gcc --version
```

Expected: mingw-w64 version 字符串

- [ ] **Step 3: 验证能交叉编译最小 hello world**

```bash
mkdir -p /tmp/hello-win && cd /tmp/hello-win
cat > main.go <<'EOF'
package main
import "fmt"
func main() { fmt.Println("hello") }
EOF
go mod init hello
GOOS=windows GOARCH=amd64 CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc go build -o hello.exe .
file hello.exe
```

Expected: `hello.exe: PE32+ executable (console) x86-64, for MS Windows`

- [ ] **Step 4: 清理**

```bash
rm -rf /tmp/hello-win /tmp/go1.22.5.linux-amd64.tar.gz
```

- [ ] **Step 5: 记录**

本任务不产生可提交代码。继续 Task 1。

---

## Task 1: 项目脚手架 + git 初始化

**Files:**
- Create: `/root/windows-auto-stimulator/app/go.mod`
- Create: `/root/windows-auto-stimulator/app/main.go`
- Create: `/root/windows-auto-stimulator/.gitignore`

- [ ] **Step 1: git 初始化仓库**

```bash
cd /root/windows-auto-stimulator
git init
git add docs/
git commit -m "docs: add spec and plan for tether"
```

- [ ] **Step 2: 创建 `.gitignore`**

```
app/tether*
app/keys/
app/config.json
*.exe
*.log
/tmp/
```

保存到 `/root/windows-auto-stimulator/.gitignore`。

- [ ] **Step 3: 初始化 Go module**

```bash
mkdir -p /root/windows-auto-stimulator/app
cd /root/windows-auto-stimulator/app
go mod init tether
```

- [ ] **Step 4: 创建最小 `main.go`**

```go
package main

import "fmt"

func main() {
	fmt.Println("tether starting...")
}
```

保存到 `/root/windows-auto-stimulator/app/main.go`。

- [ ] **Step 5: 验证构建 + 运行**

```bash
cd /root/windows-auto-stimulator/app
go build -o tether .
./tether
```

Expected: 输出 `tether starting...`

- [ ] **Step 6: 提交**

```bash
cd /root/windows-auto-stimulator
git add .gitignore app/go.mod app/main.go
git commit -m "chore: init tether go module"
```

---

## Task 2: Config 包

**Files:**
- Create: `/root/windows-auto-stimulator/app/config/config.go`
- Create: `/root/windows-auto-stimulator/app/config/config_test.go`

- [ ] **Step 1: 写失败测试**

保存到 `/root/windows-auto-stimulator/app/config/config_test.go`：

```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	orig := &Config{
		VpsHost:          "1.2.3.4",
		VpsUser:          "root",
		VpsPort:          22,
		SharePath:        `D:\proj`,
		RemoteMountPoint: "~/local-code",
		RemoteTunnelPort: 2222,
		AdminKeyPath:     "./keys/admin_ed25519",
		Bootstrapped:     true,
	}
	if err := Save(path, orig); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded.VpsHost != orig.VpsHost || loaded.VpsPort != orig.VpsPort {
		t.Errorf("round trip mismatch: %+v vs %+v", loaded, orig)
	}
}

func TestLoadMissingReturnsDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nope.json")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load missing should not err, got %v", err)
	}
	if cfg.Bootstrapped {
		t.Errorf("default should be not bootstrapped")
	}
	if cfg.VpsPort != 22 {
		t.Errorf("default VpsPort should be 22, got %d", cfg.VpsPort)
	}
	_ = os.Remove(path)
}
```

- [ ] **Step 2: 运行测试确认失败**

```bash
cd /root/windows-auto-stimulator/app
go test ./config/...
```

Expected: FAIL with `undefined: Config / Load / Save`

- [ ] **Step 3: 写最小实现**

保存到 `/root/windows-auto-stimulator/app/config/config.go`：

```go
package config

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
)

type Config struct {
	VpsHost          string `json:"vpsHost"`
	VpsUser          string `json:"vpsUser"`
	VpsPort          int    `json:"vpsPort"`
	SharePath        string `json:"sharePath"`
	RemoteMountPoint string `json:"remoteMountPoint"`
	RemoteTunnelPort int    `json:"remoteTunnelPort"`
	AdminKeyPath     string `json:"adminKeyPath"`
	Bootstrapped     bool   `json:"bootstrapped"`
}

func defaults() *Config {
	return &Config{
		VpsUser:          "root",
		VpsPort:          22,
		RemoteMountPoint: "~/local-code",
		RemoteTunnelPort: 2222,
		AdminKeyPath:     "./keys/admin_ed25519",
	}
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return defaults(), nil
	}
	if err != nil {
		return nil, err
	}
	cfg := defaults()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func Save(path string, cfg *Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
```

- [ ] **Step 4: 运行测试确认通过**

```bash
cd /root/windows-auto-stimulator/app
go test ./config/... -v
```

Expected: `PASS` on `TestSaveAndLoad` and `TestLoadMissingReturnsDefaults`

- [ ] **Step 5: 提交**

```bash
cd /root/windows-auto-stimulator
git add app/config/
git commit -m "feat(config): add Config struct with Load/Save"
```

---

## Task 3: Identity 包 (Ed25519 密钥)

**Files:**
- Create: `/root/windows-auto-stimulator/app/identity/identity.go`
- Create: `/root/windows-auto-stimulator/app/identity/identity_test.go`

- [ ] **Step 1: 添加依赖**

```bash
cd /root/windows-auto-stimulator/app
go get golang.org/x/crypto/ssh
```

- [ ] **Step 2: 写失败测试**

保存到 `/root/windows-auto-stimulator/app/identity/identity_test.go`：

```go
package identity

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test_ed25519")

	id1, err := GenerateOrLoad(path)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.HasPrefix(id1.AuthorizedKey(), "ssh-ed25519 ") {
		t.Errorf("authorized_keys format wrong: %q", id1.AuthorizedKey())
	}

	id2, err := GenerateOrLoad(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if id1.AuthorizedKey() != id2.AuthorizedKey() {
		t.Errorf("second load should return same key")
	}
}

func TestEphemeral(t *testing.T) {
	id, err := Ephemeral()
	if err != nil {
		t.Fatalf("ephemeral: %v", err)
	}
	if len(id.PrivatePEM()) == 0 {
		t.Error("ephemeral private key empty")
	}
}
```

- [ ] **Step 3: 运行测试确认失败**

```bash
go test ./identity/...
```

Expected: FAIL with undefined identifiers

- [ ] **Step 4: 写实现**

保存到 `/root/windows-auto-stimulator/app/identity/identity.go`：

```go
package identity

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
)

type Identity struct {
	priv ed25519.PrivateKey
	pub  ed25519.PublicKey
}

func GenerateOrLoad(path string) (*Identity, error) {
	if data, err := os.ReadFile(path); err == nil {
		block, _ := pem.Decode(data)
		if block == nil {
			return nil, os.ErrInvalid
		}
		priv := ed25519.PrivateKey(block.Bytes)
		return &Identity{priv: priv, pub: priv.Public().(ed25519.PublicKey)}, nil
	}
	return generateAndSave(path)
}

func Ephemeral() (*Identity, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return &Identity{priv: priv, pub: pub}, nil
}

func generateAndSave(path string) (*Identity, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	block := &pem.Block{Type: "ED25519 PRIVATE KEY", Bytes: priv}
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
		return nil, err
	}
	return &Identity{priv: priv, pub: pub}, nil
}

func (i *Identity) Signer() (ssh.Signer, error) {
	return ssh.NewSignerFromKey(i.priv)
}

func (i *Identity) PublicKey() ssh.PublicKey {
	pk, _ := ssh.NewPublicKey(i.pub)
	return pk
}

func (i *Identity) AuthorizedKey() string {
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(i.PublicKey())))
}

func (i *Identity) PrivatePEM() []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "ED25519 PRIVATE KEY", Bytes: i.priv})
}
```

- [ ] **Step 5: 运行测试确认通过**

```bash
go test ./identity/... -v
```

Expected: PASS both tests

- [ ] **Step 6: 提交**

```bash
cd /root/windows-auto-stimulator
git add app/identity/ app/go.mod app/go.sum
git commit -m "feat(identity): add Ed25519 key generate/load"
```

---

## Task 4: SFTP Server 基础

**Files:**
- Create: `/root/windows-auto-stimulator/app/sftpserver/server.go`
- Create: `/root/windows-auto-stimulator/app/sftpserver/server_test.go`

- [ ] **Step 1: 添加依赖**

```bash
cd /root/windows-auto-stimulator/app
go get github.com/pkg/sftp
```

- [ ] **Step 2: 写集成测试**

保存到 `/root/windows-auto-stimulator/app/sftpserver/server_test.go`：

```go
package sftpserver

import (
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"tether/identity"
)

func TestSFTPServerRoundTrip(t *testing.T) {
	// 准备要服务的目录
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("world"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 启动服务端
	serverID, _ := identity.Ephemeral()
	clientID, _ := identity.Ephemeral()

	srv, err := Start(Config{
		RootDir:        dir,
		HostIdentity:   serverID,
		AllowedClientKey: clientID.PublicKey(),
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Close()

	// 客户端连过去读文件
	clientSigner, _ := clientID.Signer()
	cfg := &ssh.ClientConfig{
		User:            "winshare",
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(clientSigner)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	conn, err := ssh.Dial("tcp", srv.Addr(), cfg)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	sc, err := sftp.NewClient(conn)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer sc.Close()

	f, err := sc.Open("hello.txt")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()
	content, _ := io.ReadAll(f)
	if string(content) != "world" {
		t.Errorf("got %q, want %q", content, "world")
	}
}

func TestSFTPServerRejectsWrongKey(t *testing.T) {
	dir := t.TempDir()
	serverID, _ := identity.Ephemeral()
	allowedID, _ := identity.Ephemeral()
	wrongID, _ := identity.Ephemeral()

	srv, err := Start(Config{
		RootDir:          dir,
		HostIdentity:     serverID,
		AllowedClientKey: allowedID.PublicKey(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	wrongSigner, _ := wrongID.Signer()
	cfg := &ssh.ClientConfig{
		User:            "winshare",
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(wrongSigner)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	_, err = ssh.Dial("tcp", srv.Addr(), cfg)
	if err == nil {
		t.Error("expected auth failure")
	}

	// sanity: listener still alive
	_ = net.Dialer{}
}
```

- [ ] **Step 3: 运行测试确认失败**

```bash
go test ./sftpserver/...
```

Expected: FAIL (undefined Start/Config etc.)

- [ ] **Step 4: 写实现**

保存到 `/root/windows-auto-stimulator/app/sftpserver/server.go`：

```go
package sftpserver

import (
	"bytes"
	"fmt"
	"net"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"tether/identity"
)

type Config struct {
	RootDir          string
	HostIdentity     *identity.Identity
	AllowedClientKey ssh.PublicKey
}

type Server struct {
	listener net.Listener
	cfg      Config
	done     chan struct{}
}

func Start(cfg Config) (*Server, error) {
	sshCfg := &ssh.ServerConfig{
		PublicKeyCallback: func(meta ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if bytes.Equal(key.Marshal(), cfg.AllowedClientKey.Marshal()) {
				return &ssh.Permissions{}, nil
			}
			return nil, fmt.Errorf("unauthorized key")
		},
	}
	hostSigner, err := cfg.HostIdentity.Signer()
	if err != nil {
		return nil, err
	}
	sshCfg.AddHostKey(hostSigner)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	s := &Server{listener: ln, cfg: cfg, done: make(chan struct{})}
	go s.serve(sshCfg)
	return s, nil
}

func (s *Server) Addr() string {
	return s.listener.Addr().String()
}

func (s *Server) Port() int {
	return s.listener.Addr().(*net.TCPAddr).Port
}

func (s *Server) Close() error {
	close(s.done)
	return s.listener.Close()
}

func (s *Server) serve(sshCfg *ssh.ServerConfig) {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.done:
				return
			default:
			}
			return
		}
		go s.handleConn(conn, sshCfg)
	}
}

func (s *Server) handleConn(nConn net.Conn, sshCfg *ssh.ServerConfig) {
	defer nConn.Close()
	_, chans, reqs, err := ssh.NewServerConn(nConn, sshCfg)
	if err != nil {
		return
	}
	go ssh.DiscardRequests(reqs)
	for newChan := range chans {
		if newChan.ChannelType() != "session" {
			_ = newChan.Reject(ssh.UnknownChannelType, "unknown channel")
			continue
		}
		ch, reqs, err := newChan.Accept()
		if err != nil {
			continue
		}
		go func(ch ssh.Channel, reqs <-chan *ssh.Request) {
			defer ch.Close()
			for req := range reqs {
				ok := false
				if req.Type == "subsystem" && len(req.Payload) >= 4 && string(req.Payload[4:]) == "sftp" {
					ok = true
					_ = req.Reply(true, nil)
					srv, err := sftp.NewServer(ch, sftp.WithServerWorkingDirectory(s.cfg.RootDir))
					if err != nil {
						return
					}
					_ = srv.Serve()
					return
				}
				_ = req.Reply(ok, nil)
			}
		}(ch, reqs)
	}
}
```

- [ ] **Step 5: 运行测试确认通过**

```bash
go test ./sftpserver/... -v
```

Expected: `PASS` on both tests

- [ ] **Step 6: 提交**

```bash
cd /root/windows-auto-stimulator
git add app/sftpserver/ app/go.mod app/go.sum
git commit -m "feat(sftpserver): embedded SFTP server with pubkey auth"
```

---

## Task 5: SFTP Server 目录根限制

**Files:**
- Modify: `/root/windows-auto-stimulator/app/sftpserver/server.go`
- Modify: `/root/windows-auto-stimulator/app/sftpserver/server_test.go`

**说明**：`sftp.WithServerWorkingDirectory` 只设置客户端初始目录，**不限制**客户端能访问的范围（客户端可以 `cd /` 逃逸）。我们需要一个自定义的 `Handlers` 把所有路径强制 chroot 到 RootDir。

- [ ] **Step 1: 加一个逃逸测试**

追加到 `/root/windows-auto-stimulator/app/sftpserver/server_test.go`：

```go
func TestSFTPServerRejectsEscape(t *testing.T) {
	dir := t.TempDir()
	serverID, _ := identity.Ephemeral()
	clientID, _ := identity.Ephemeral()

	srv, _ := Start(Config{
		RootDir:          dir,
		HostIdentity:     serverID,
		AllowedClientKey: clientID.PublicKey(),
	})
	defer srv.Close()

	signer, _ := clientID.Signer()
	conn, _ := ssh.Dial("tcp", srv.Addr(), &ssh.ClientConfig{
		User:            "winshare",
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	})
	defer conn.Close()
	sc, _ := sftp.NewClient(conn)
	defer sc.Close()

	// 尝试读根目录外的文件
	_, err := sc.Open("/etc/passwd")
	if err == nil {
		t.Error("expected permission denied when escaping root")
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

```bash
go test ./sftpserver/...
```

Expected: `TestSFTPServerRejectsEscape` FAIL

- [ ] **Step 3: 实现 chroot Handlers**

替换 `server.go` 中 `handleConn` 里的 sftp 处理部分，改为用自定义 Handlers：

确保 server.go 顶部 import 包含 `io`, `os`, `path/filepath`, `strings`（原文件已有的保留）。

在文件末尾追加：

```go
type chrootHandlers struct {
	root string
}

func (c *chrootHandlers) realPath(p string) (string, error) {
	clean := filepath.Clean("/" + strings.TrimPrefix(p, "/"))
	full := filepath.Join(c.root, clean)
	rel, err := filepath.Rel(c.root, full)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", os.ErrPermission
	}
	return full, nil
}

func (c *chrootHandlers) Fileread(r *sftp.Request) (io.ReaderAt, error) {
	full, err := c.realPath(r.Filepath)
	if err != nil {
		return nil, err
	}
	return os.OpenFile(full, os.O_RDONLY, 0)
}

func (c *chrootHandlers) Filewrite(r *sftp.Request) (io.WriterAt, error) {
	full, err := c.realPath(r.Filepath)
	if err != nil {
		return nil, err
	}
	return os.OpenFile(full, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
}

func (c *chrootHandlers) Filecmd(r *sftp.Request) error {
	full, err := c.realPath(r.Filepath)
	if err != nil {
		return err
	}
	switch r.Method {
	case "Setstat":
		return nil
	case "Rename":
		target, err := c.realPath(r.Target)
		if err != nil {
			return err
		}
		return os.Rename(full, target)
	case "Rmdir":
		return os.Remove(full)
	case "Mkdir":
		return os.MkdirAll(full, 0o755)
	case "Remove":
		return os.Remove(full)
	case "Symlink":
		return os.ErrPermission // 禁用符号链接创建 (防逃逸)
	}
	return nil
}

type listerAt []os.FileInfo

func (l listerAt) ListAt(f []os.FileInfo, offset int64) (int, error) {
	if offset >= int64(len(l)) {
		return 0, io.EOF
	}
	n := copy(f, l[offset:])
	if n < len(f) {
		return n, io.EOF
	}
	return n, nil
}

func (c *chrootHandlers) Filelist(r *sftp.Request) (sftp.ListerAt, error) {
	full, err := c.realPath(r.Filepath)
	if err != nil {
		return nil, err
	}
	switch r.Method {
	case "List":
		entries, err := os.ReadDir(full)
		if err != nil {
			return nil, err
		}
		infos := make([]os.FileInfo, 0, len(entries))
		for _, e := range entries {
			info, err := e.Info()
			if err == nil {
				infos = append(infos, info)
			}
		}
		return listerAt(infos), nil
	case "Stat":
		info, err := os.Stat(full)
		if err != nil {
			return nil, err
		}
		return listerAt([]os.FileInfo{info}), nil
	}
	return nil, os.ErrInvalid
}

```

注意 server.go 顶部 import 不需要 `sync`，只要新增 `io`, `os`, `path/filepath`, `strings`（如已有则跳过）。

然后修改 `handleConn` 里处理 `subsystem sftp` 的部分：

```go
if req.Type == "subsystem" && len(req.Payload) >= 4 && string(req.Payload[4:]) == "sftp" {
    ok = true
    _ = req.Reply(true, nil)
    handlers := &chrootHandlers{root: s.cfg.RootDir}
    sftpHandlers := sftp.Handlers{
        FileGet:  handlers,
        FilePut:  handlers,
        FileCmd:  handlers,
        FileList: handlers,
    }
    srv := sftp.NewRequestServer(ch, sftpHandlers)
    _ = srv.Serve()
    srv.Close()
    return
}
```

- [ ] **Step 4: 运行全部 sftpserver 测试**

```bash
go test ./sftpserver/... -v
```

Expected: 三个测试全 PASS

- [ ] **Step 5: 提交**

```bash
cd /root/windows-auto-stimulator
git add app/sftpserver/
git commit -m "feat(sftpserver): chroot handlers prevent path escape"
```

---

## Task 6: SSH client - 拨号

**Files:**
- Create: `/root/windows-auto-stimulator/app/sshclient/client.go`
- Create: `/root/windows-auto-stimulator/app/sshclient/client_test.go`

**说明**：对 SSH client 做端到端单元测试需要一个真实的 ssh server。我们用 Go 的 `crypto/ssh` 在测试里起一个 in-process server，验证 `Dial` 能用密码或密钥成功登录。

- [ ] **Step 1: 写失败测试**

保存到 `/root/windows-auto-stimulator/app/sshclient/client_test.go`：

```go
package sshclient

import (
	"fmt"
	"net"
	"testing"

	"golang.org/x/crypto/ssh"

	"tether/identity"
)

// startTestSSHServer 在 127.0.0.1 起一个只接受固定密码或公钥的 SSH 服务端,
// 返回监听地址和关闭函数。
func startTestSSHServer(t *testing.T, password string, allowed ssh.PublicKey) (string, func()) {
	t.Helper()
	hostID, _ := identity.Ephemeral()
	hostSigner, _ := hostID.Signer()

	cfg := &ssh.ServerConfig{
		PasswordCallback: func(conn ssh.ConnMetadata, pw []byte) (*ssh.Permissions, error) {
			if string(pw) == password {
				return &ssh.Permissions{}, nil
			}
			return nil, ssh.ErrNoAuth
		},
		PublicKeyCallback: func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if allowed != nil && string(key.Marshal()) == string(allowed.Marshal()) {
				return &ssh.Permissions{}, nil
			}
			return nil, ssh.ErrNoAuth
		},
	}
	cfg.AddHostKey(hostSigner)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				_, chans, reqs, err := ssh.NewServerConn(conn, cfg)
				if err != nil {
					return
				}
				go ssh.DiscardRequests(reqs)
				for newCh := range chans {
					_ = newCh.Reject(ssh.UnknownChannelType, "no sessions in test")
				}
			}()
		}
	}()

	return ln.Addr().String(), func() { ln.Close() }
}

func splitHostPort(t *testing.T, addr string) (string, int) {
	t.Helper()
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	var p int
	if _, err := fmt.Sscanf(portStr, "%d", &p); err != nil {
		t.Fatal(err)
	}
	return host, p
}

func TestDialWithPassword(t *testing.T) {
	addr, stop := startTestSSHServer(t, "secret", nil)
	defer stop()
	host, port := splitHostPort(t, addr)

	c, err := DialPassword(host, port, "testuser", "secret")
	if err != nil {
		t.Fatalf("DialPassword: %v", err)
	}
	defer c.Close()
}

func TestDialWithKey(t *testing.T) {
	clientID, _ := identity.Ephemeral()
	addr, stop := startTestSSHServer(t, "", clientID.PublicKey())
	defer stop()
	host, port := splitHostPort(t, addr)

	c, err := DialKey(host, port, "testuser", clientID)
	if err != nil {
		t.Fatalf("DialKey: %v", err)
	}
	defer c.Close()
}
```

- [ ] **Step 2: 运行测试确认失败**

```bash
go test ./sshclient/...
```

Expected: FAIL (undefined `DialPassword` / `DialKey`)

- [ ] **Step 3: 写实现**

保存到 `/root/windows-auto-stimulator/app/sshclient/client.go`：

```go
package sshclient

import (
	"fmt"
	"time"

	"golang.org/x/crypto/ssh"

	"tether/identity"
)

type Client struct {
	conn *ssh.Client
}

func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

func (c *Client) Raw() *ssh.Client { return c.conn }

func DialPassword(host string, port int, user, password string) (*Client, error) {
	cfg := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{ssh.Password(password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}
	return dial(host, port, cfg)
}

func DialKey(host string, port int, user string, id *identity.Identity) (*Client, error) {
	signer, err := id.Signer()
	if err != nil {
		return nil, err
	}
	cfg := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}
	return dial(host, port, cfg)
}

func dial(host string, port int, cfg *ssh.ClientConfig) (*Client, error) {
	addr := fmt.Sprintf("%s:%d", host, port)
	conn, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		return nil, err
	}
	return &Client{conn: conn}, nil
}
```

- [ ] **Step 4: 运行测试确认通过**

```bash
go test ./sshclient/... -v
```

Expected: PASS both tests

- [ ] **Step 5: 提交**

```bash
cd /root/windows-auto-stimulator
git add app/sshclient/
git commit -m "feat(sshclient): add DialPassword and DialKey"
```

---

## Task 7: SSH client - 远程命令封装

**Files:**
- Modify: `/root/windows-auto-stimulator/app/sshclient/client.go`
- Create: `/root/windows-auto-stimulator/app/sshclient/remote.go`
- Modify: `/root/windows-auto-stimulator/app/sshclient/client_test.go`

- [ ] **Step 1: 扩展测试服务端以支持 session+exec**

修改 `client_test.go` 里的 `startTestSSHServer`，把 session 通道从"reject"改为接受 exec 并回显固定字符串。追加：

```go
func startTestSSHServerWithExec(t *testing.T, password string) (string, func()) {
	t.Helper()
	hostID, _ := identity.Ephemeral()
	hostSigner, _ := hostID.Signer()

	cfg := &ssh.ServerConfig{
		PasswordCallback: func(conn ssh.ConnMetadata, pw []byte) (*ssh.Permissions, error) {
			if string(pw) == password {
				return &ssh.Permissions{}, nil
			}
			return nil, ssh.ErrNoAuth
		},
	}
	cfg.AddHostKey(hostSigner)

	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				_, chans, reqs, err := ssh.NewServerConn(conn, cfg)
				if err != nil {
					return
				}
				go ssh.DiscardRequests(reqs)
				for newCh := range chans {
					if newCh.ChannelType() != "session" {
						_ = newCh.Reject(ssh.UnknownChannelType, "")
						continue
					}
					ch, reqs, _ := newCh.Accept()
					go func(ch ssh.Channel, reqs <-chan *ssh.Request) {
						defer ch.Close()
						for req := range reqs {
							if req.Type == "exec" {
								// payload: 4 bytes length + command
								_ = req.Reply(true, nil)
								ch.Write([]byte("OK\n"))
								// exit-status
								ch.SendRequest("exit-status", false, []byte{0, 0, 0, 0})
								return
							}
							_ = req.Reply(false, nil)
						}
					}(ch, reqs)
				}
			}()
		}
	}()
	return ln.Addr().String(), func() { ln.Close() }
}

func TestRunCommand(t *testing.T) {
	addr, stop := startTestSSHServerWithExec(t, "pw")
	defer stop()
	host, port := splitHostPort(t, addr)

	c, err := DialPassword(host, port, "u", "pw")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	out, err := c.Run("echo hello")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if string(out) != "OK\n" {
		t.Errorf("got %q", out)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

```bash
go test ./sshclient/...
```

Expected: FAIL (undefined `c.Run`)

- [ ] **Step 3: 写实现**

保存到 `/root/windows-auto-stimulator/app/sshclient/remote.go`：

```go
package sshclient

import (
	"bytes"
	"fmt"
)

func (c *Client) Run(cmd string) ([]byte, error) {
	session, err := c.conn.NewSession()
	if err != nil {
		return nil, err
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr
	if err := session.Run(cmd); err != nil {
		return stdout.Bytes(), fmt.Errorf("run %q: %w; stderr=%s", cmd, err, stderr.String())
	}
	return stdout.Bytes(), nil
}

// RunScript 把一段多行 bash 脚本通过 base64 安全传输并执行
func (c *Client) RunScript(script string) ([]byte, error) {
	encoded := base64Encode(script)
	return c.Run(fmt.Sprintf("echo %s | base64 -d | bash", encoded))
}

func base64Encode(s string) string {
	return encodeBase64Std(s)
}
```

在同一文件加一个私有 base64 编码（避免多写 import）：

```go
import "encoding/base64"

func encodeBase64Std(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}
```

合并后的完整 `remote.go`：

```go
package sshclient

import (
	"bytes"
	"encoding/base64"
	"fmt"
)

func (c *Client) Run(cmd string) ([]byte, error) {
	session, err := c.conn.NewSession()
	if err != nil {
		return nil, err
	}
	defer session.Close()
	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr
	if err := session.Run(cmd); err != nil {
		return stdout.Bytes(), fmt.Errorf("run %q: %w; stderr=%s", cmd, err, stderr.String())
	}
	return stdout.Bytes(), nil
}

func (c *Client) RunScript(script string) ([]byte, error) {
	encoded := base64.StdEncoding.EncodeToString([]byte(script))
	return c.Run(fmt.Sprintf("echo %s | base64 -d | bash", encoded))
}
```

- [ ] **Step 4: 运行测试确认通过**

```bash
go test ./sshclient/... -v
```

Expected: PASS all tests

- [ ] **Step 5: 提交**

```bash
cd /root/windows-auto-stimulator
git add app/sshclient/
git commit -m "feat(sshclient): add Run and RunScript helpers"
```

---

## Task 8: SSH client - 反向端口转发

**Files:**
- Modify: `/root/windows-auto-stimulator/app/sshclient/client.go`
- Modify: `/root/windows-auto-stimulator/app/sshclient/client_test.go`

**说明**：`ssh.Client.Listen("tcp", "localhost:2222")` 会让 SSH server 在远端打开这个端口，本地每次 `Accept()` 到的连接就是从远端来的。我们把这些连接代理到 exe 本地的 SFTP Server 端口。

- [ ] **Step 1: 写测试**

追加到 `client_test.go`：

```go
func TestReverseForward(t *testing.T) {
	// 这里用同一个 exec-enabled test server (它底层支持 tcpip-forward 请求吗?)
	// crypto/ssh 的 server 默认不处理 tcpip-forward. 这个测试跳过真实转发路径,
	// 直接验证 ReverseForward 不会 panic 且失败时返回 error.
	t.Skip("reverse forward integration test requires real sshd; covered by manual E2E")
}
```

保留占位跳过，真实用 VPS 作为集成环境验证。

- [ ] **Step 2: 写实现**

追加到 `client.go`：

```go
import "io"

// ReverseForward 让 VPS 在 remoteAddr (例如 "localhost:2222") 打开一个监听端口,
// 每次远端收到的连接都转发到本地 localAddr.
// 返回一个关闭函数, 调用后停止转发并关闭远端监听.
func (c *Client) ReverseForward(remoteAddr, localAddr string) (func(), error) {
	ln, err := c.conn.Listen("tcp", remoteAddr)
	if err != nil {
		return nil, fmt.Errorf("remote Listen %s: %w", remoteAddr, err)
	}
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			default:
			}
			rConn, err := ln.Accept()
			if err != nil {
				return
			}
			go proxyConn(rConn, localAddr)
		}
	}()
	return func() {
		close(done)
		ln.Close()
	}, nil
}

func proxyConn(remote io.ReadWriteCloser, localAddr string) {
	defer remote.Close()
	local, err := netDial("tcp", localAddr)
	if err != nil {
		return
	}
	defer local.Close()
	errc := make(chan error, 2)
	go func() { _, err := io.Copy(remote, local); errc <- err }()
	go func() { _, err := io.Copy(local, remote); errc <- err }()
	<-errc
}

// netDial 间接引用以便测试时替换, 当前是直接调用 net.Dial
func netDial(network, addr string) (io.ReadWriteCloser, error) {
	return net.Dial(network, addr)
}
```

需要给 client.go 加 `import "net"` 如果还没有。

- [ ] **Step 3: 运行测试**

```bash
go test ./sshclient/... -v
```

Expected: PASS with `TestReverseForward` SKIP

- [ ] **Step 4: 提交**

```bash
cd /root/windows-auto-stimulator
git add app/sshclient/
git commit -m "feat(sshclient): add ReverseForward for ssh -R"
```

---

## Task 9: Remote - bootstrap 脚本

**Files:**
- Create: `/root/windows-auto-stimulator/app/sshclient/bootstrap.go`

（无单元测试，靠 Task 19 的 E2E 验证）

- [ ] **Step 1: 写实现**

保存到 `/root/windows-auto-stimulator/app/sshclient/bootstrap.go`：

```go
package sshclient

import (
	"fmt"
	"strings"
)

// Bootstrap 在 VPS 上做首次配置: 装 sshfs, 把 adminPubKey 加入 authorized_keys.
// 必须通过密码 SSH 调用.
func (c *Client) Bootstrap(adminPubKey string) error {
	pub := strings.TrimSpace(adminPubKey)
	if !strings.HasPrefix(pub, "ssh-ed25519 ") {
		return fmt.Errorf("invalid admin pubkey")
	}
	script := fmt.Sprintf(`
set -e
mkdir -p ~/.ssh && chmod 700 ~/.ssh
touch ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys
grep -qxF %q ~/.ssh/authorized_keys || echo %q >> ~/.ssh/authorized_keys
if ! command -v sshfs >/dev/null 2>&1; then
  if command -v apt-get >/dev/null 2>&1; then
    apt-get update -qq >/dev/null 2>&1 || true
    DEBIAN_FRONTEND=noninteractive apt-get install -y sshfs >/dev/null
  elif command -v yum >/dev/null 2>&1; then
    yum install -y fuse-sshfs >/dev/null
  else
    echo "no package manager found" >&2
    exit 1
  fi
fi
command -v sshfs >/dev/null 2>&1 || { echo "sshfs install failed" >&2; exit 1; }
echo bootstrap-ok
`, pub, pub)
	out, err := c.RunScript(script)
	if err != nil {
		return fmt.Errorf("bootstrap failed: %w", err)
	}
	if !strings.Contains(string(out), "bootstrap-ok") {
		return fmt.Errorf("bootstrap output unexpected: %s", out)
	}
	return nil
}
```

- [ ] **Step 2: 验证编译**

```bash
cd /root/windows-auto-stimulator/app
go build ./...
```

Expected: 无报错

- [ ] **Step 3: 提交**

```bash
cd /root/windows-auto-stimulator
git add app/sshclient/bootstrap.go
git commit -m "feat(sshclient): add Bootstrap for first-time VPS setup"
```

---

## Task 10: Remote - mount/unmount

**Files:**
- Create: `/root/windows-auto-stimulator/app/sshclient/mount.go`

- [ ] **Step 1: 写实现**

保存到 `/root/windows-auto-stimulator/app/sshclient/mount.go`：

```go
package sshclient

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
)

type MountParams struct {
	WinShareUser      string // 固定为 "winshare"
	RemoteTunnelPort  int    // 对应 ReverseForward 在远端打开的端口
	RemoteMountPoint  string // 例如 ~/local-code
	SFTPPrivateKeyPEM []byte // 临时 sftp 私钥内容
}

type MountHandle struct {
	TempKeyPath string
	MountPoint  string
}

func (c *Client) Mount(p MountParams) (*MountHandle, error) {
	randBytes := make([]byte, 8)
	if _, err := rand.Read(randBytes); err != nil {
		return nil, err
	}
	tempKey := fmt.Sprintf("/tmp/.tether-%s", hex.EncodeToString(randBytes))

	// 把私钥内容和挂载命令打包到一个脚本, RunScript 统一执行
	script := fmt.Sprintf(`
set -e
umask 077
cat > %s <<'KEOF'
%s
KEOF
chmod 600 %s
mkdir -p %s
fusermount -u %s 2>/dev/null || true
sshfs -p %d \
  -o IdentityFile=%s \
  -o StrictHostKeyChecking=no \
  -o UserKnownHostsFile=/dev/null \
  -o reconnect,ServerAliveInterval=15,ServerAliveCountMax=3 \
  -o cache=yes,compression=no \
  %s@localhost:/ %s
echo mount-ok
`,
		tempKey, strings.TrimRight(string(p.SFTPPrivateKeyPEM), "\n"),
		tempKey,
		p.RemoteMountPoint, p.RemoteMountPoint,
		p.RemoteTunnelPort, tempKey,
		p.WinShareUser, p.RemoteMountPoint,
	)

	out, err := c.RunScript(script)
	if err != nil {
		return nil, fmt.Errorf("mount failed: %w (output: %s)", err, out)
	}
	if !strings.Contains(string(out), "mount-ok") {
		return nil, fmt.Errorf("mount unexpected output: %s", out)
	}
	return &MountHandle{TempKeyPath: tempKey, MountPoint: p.RemoteMountPoint}, nil
}

func (c *Client) Unmount(h *MountHandle) error {
	if h == nil {
		return nil
	}
	script := fmt.Sprintf(`
fusermount -u %s 2>/dev/null || true
rm -f %s
echo unmount-ok
`, h.MountPoint, h.TempKeyPath)
	out, err := c.RunScript(script)
	if err != nil {
		return fmt.Errorf("unmount failed: %w (output: %s)", err, out)
	}
	if !strings.Contains(string(out), "unmount-ok") {
		return fmt.Errorf("unmount unexpected output: %s", out)
	}
	return nil
}
```

- [ ] **Step 2: 验证编译**

```bash
cd /root/windows-auto-stimulator/app
go build ./...
```

Expected: 无报错

- [ ] **Step 3: 提交**

```bash
cd /root/windows-auto-stimulator
git add app/sshclient/mount.go
git commit -m "feat(sshclient): add Mount and Unmount"
```

---

## Task 11: Claude 启动器

**Files:**
- Create: `/root/windows-auto-stimulator/app/claude/launcher.go`

**说明**：仅在 Windows 上有意义；Linux/Mac 构建时我们让函数返回错误。

- [ ] **Step 1: 写实现 (Windows)**

保存到 `/root/windows-auto-stimulator/app/claude/launcher_windows.go`：

```go
//go:build windows

package claude

import (
	"fmt"
	"os/exec"
	"syscall"
)

type LaunchParams struct {
	AdminKeyPath     string
	VpsHost          string
	VpsUser          string
	VpsPort          int
	RemoteMountPoint string
}

func Launch(p LaunchParams) error {
	// 新开一个 PowerShell 窗口, 在里面 ssh 进 VPS 执行 claude
	sshArgs := fmt.Sprintf(
		`ssh -i "%s" -p %d -o StrictHostKeyChecking=no -t %s@%s "cd %s && claude"`,
		p.AdminKeyPath, p.VpsPort, p.VpsUser, p.VpsHost, p.RemoteMountPoint,
	)
	psCmd := fmt.Sprintf(`Start-Process powershell -ArgumentList '-NoExit','-Command','%s'`, sshArgs)
	cmd := exec.Command("powershell.exe", "-NoProfile", "-Command", psCmd)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Start()
}
```

保存到 `/root/windows-auto-stimulator/app/claude/launcher_other.go`：

```go
//go:build !windows

package claude

import "errors"

type LaunchParams struct {
	AdminKeyPath     string
	VpsHost          string
	VpsUser          string
	VpsPort          int
	RemoteMountPoint string
}

func Launch(p LaunchParams) error {
	return errors.New("claude launcher only supported on windows")
}
```

- [ ] **Step 2: 验证 Linux 编译 (占位)**

```bash
cd /root/windows-auto-stimulator/app
go build ./...
```

Expected: 无报错（Linux 构建会使用 `launcher_other.go`）

- [ ] **Step 3: 提交**

```bash
cd /root/windows-auto-stimulator
git add app/claude/
git commit -m "feat(claude): add Windows PowerShell launcher"
```

---

## Task 12: UI - Fyne 骨架 + 主窗口

**Files:**
- Create: `/root/windows-auto-stimulator/app/ui/app.go`
- Modify: `/root/windows-auto-stimulator/app/main.go`

- [ ] **Step 1: 添加 Fyne 依赖**

```bash
cd /root/windows-auto-stimulator/app
go get fyne.io/fyne/v2@v2.5.2
go get fyne.io/fyne/v2/app
go get fyne.io/fyne/v2/container
go get fyne.io/fyne/v2/widget
```

- [ ] **Step 2: 写 `ui/app.go`**

```go
package ui

import (
	"fyne.io/fyne/v2"
	fyneapp "fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

type UI struct {
	app    fyne.App
	window fyne.Window
}

func New() *UI {
	a := fyneapp.New()
	w := a.NewWindow("tether")
	w.Resize(fyne.NewSize(560, 560))
	u := &UI{app: a, window: w}
	u.build()
	return u
}

func (u *UI) build() {
	placeholder := widget.NewLabel("tether starting...")
	u.window.SetContent(container.NewCenter(placeholder))
}

func (u *UI) Run() {
	u.window.ShowAndRun()
}
```

- [ ] **Step 3: 修改 `main.go`**

替换 `main.go` 全部内容：

```go
package main

import "tether/ui"

func main() {
	ui.New().Run()
}
```

- [ ] **Step 4: 验证构建 (Linux)**

```bash
cd /root/windows-auto-stimulator/app
go build -o tether .
```

Expected: 编译成功（Linux 本地不会有 display，直接跑会卡在 `ShowAndRun`，所以不跑）

- [ ] **Step 5: 提交**

```bash
cd /root/windows-auto-stimulator
git add app/go.mod app/go.sum app/ui/ app/main.go
git commit -m "feat(ui): add Fyne main window skeleton"
```

---

## Task 13: UI - 配置表单

**Files:**
- Create: `/root/windows-auto-stimulator/app/ui/form.go`
- Modify: `/root/windows-auto-stimulator/app/ui/app.go`

- [ ] **Step 1: 写 `ui/form.go`**

```go
package ui

import (
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"tether/config"
)

type ConfigForm struct {
	cfg *config.Config

	hostEntry   *widget.Entry
	userEntry   *widget.Entry
	portEntry   *widget.Entry
	passEntry   *widget.Entry
	shareEntry  *widget.Entry
	mountEntry  *widget.Entry

	container *fyne.Container
}

func NewConfigForm(cfg *config.Config) *ConfigForm {
	f := &ConfigForm{cfg: cfg}

	f.hostEntry = widget.NewEntry()
	f.hostEntry.SetText(cfg.VpsHost)
	f.hostEntry.SetPlaceHolder("1.2.3.4")

	f.userEntry = widget.NewEntry()
	f.userEntry.SetText(cfg.VpsUser)

	f.portEntry = widget.NewEntry()
	f.portEntry.SetText(strconv.Itoa(cfg.VpsPort))

	f.passEntry = widget.NewPasswordEntry()
	f.passEntry.SetPlaceHolder("仅首次配置需要")

	f.shareEntry = widget.NewEntry()
	f.shareEntry.SetText(cfg.SharePath)
	f.shareEntry.SetPlaceHolder(`例: D:\my-project`)

	f.mountEntry = widget.NewEntry()
	f.mountEntry.SetText(cfg.RemoteMountPoint)

	form := widget.NewForm(
		widget.NewFormItem("VPS 地址", f.hostEntry),
		widget.NewFormItem("VPS 用户", f.userEntry),
		widget.NewFormItem("VPS 端口", f.portEntry),
		widget.NewFormItem("VPS 密码", f.passEntry),
		widget.NewFormItem("共享目录", f.shareEntry),
		widget.NewFormItem("远端挂载", f.mountEntry),
	)

	f.container = container.NewVBox(form)
	return f
}

func (f *ConfigForm) Container() *fyne.Container { return f.container }

// Snapshot 把 UI 字段写回 Config 并返回密码 (密码独立返回, 不存 Config)
func (f *ConfigForm) Snapshot() (password string) {
	f.cfg.VpsHost = f.hostEntry.Text
	f.cfg.VpsUser = f.userEntry.Text
	if p, err := strconv.Atoi(f.portEntry.Text); err == nil {
		f.cfg.VpsPort = p
	}
	f.cfg.SharePath = f.shareEntry.Text
	f.cfg.RemoteMountPoint = f.mountEntry.Text
	return f.passEntry.Text
}

func (f *ConfigForm) ClearPassword() { f.passEntry.SetText("") }
```

- [ ] **Step 2: 修改 `ui/app.go` 使用表单**

替换 `ui/app.go` 的 `build` 方法：

```go
func (u *UI) build() {
	cfg, _ := config.Load("./config.json")
	u.cfg = cfg
	u.form = NewConfigForm(cfg)

	content := container.NewVBox(
		widget.NewLabelWithStyle("连接配置", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		u.form.Container(),
	)
	u.window.SetContent(content)
}
```

并在 `UI` 结构体加字段：

```go
type UI struct {
	app    fyne.App
	window fyne.Window
	cfg    *config.Config
	form   *ConfigForm
}
```

加 import：

```go
import (
	"fyne.io/fyne/v2"
	fyneapp "fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"tether/config"
)
```

- [ ] **Step 3: 验证构建**

```bash
cd /root/windows-auto-stimulator/app
go build -o tether .
```

Expected: 成功

- [ ] **Step 4: 提交**

```bash
cd /root/windows-auto-stimulator
git add app/ui/
git commit -m "feat(ui): add config form"
```

---

## Task 14: UI - 状态面板 + 日志组件

**Files:**
- Create: `/root/windows-auto-stimulator/app/ui/status.go`
- Create: `/root/windows-auto-stimulator/app/ui/log.go`
- Modify: `/root/windows-auto-stimulator/app/ui/app.go`

- [ ] **Step 1: 写 `ui/status.go`**

```go
package ui

import (
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

type State int

const (
	StateDisconnected State = iota
	StateConnecting
	StateConnected
)

type StatusPanel struct {
	label     *widget.Label
	uptime    *widget.Label
	startTime time.Time
	state     State
	container *fyne.Container
}

func NewStatusPanel() *StatusPanel {
	s := &StatusPanel{
		label:  widget.NewLabel("● 未连接"),
		uptime: widget.NewLabel("运行: 00:00:00"),
	}
	s.container = container.NewHBox(s.label, widget.NewSeparator(), s.uptime)
	return s
}

func (s *StatusPanel) Container() *fyne.Container { return s.container }

func (s *StatusPanel) Set(state State) {
	s.state = state
	switch state {
	case StateDisconnected:
		s.label.SetText("● 未连接")
	case StateConnecting:
		s.label.SetText("● 连接中…")
	case StateConnected:
		s.label.SetText("● 已连接")
		s.startTime = time.Now()
		go s.tick()
	}
}

func (s *StatusPanel) tick() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for range ticker.C {
		if s.state != StateConnected {
			return
		}
		d := time.Since(s.startTime)
		s.uptime.SetText(fmt.Sprintf("运行: %02d:%02d:%02d",
			int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60))
	}
}
```

- [ ] **Step 2: 写 `ui/log.go`**

```go
package ui

import (
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

type LogPanel struct {
	entry     *widget.Entry
	container *fyne.Container
}

func NewLogPanel() *LogPanel {
	e := widget.NewMultiLineEntry()
	e.Wrapping = fyne.TextWrapWord
	e.Disable()
	scroll := container.NewVScroll(e)
	scroll.SetMinSize(fyne.NewSize(500, 150))
	return &LogPanel{
		entry:     e,
		container: container.NewStack(scroll),
	}
}

func (l *LogPanel) Container() *fyne.Container { return l.container }

func (l *LogPanel) Append(msg string) {
	ts := time.Now().Format("15:04:05")
	cur := l.entry.Text
	if cur != "" {
		cur += "\n"
	}
	l.entry.SetText(cur + fmt.Sprintf("[%s] %s", ts, msg))
}

func (l *LogPanel) Error(format string, a ...any) {
	l.Append("[ERROR] " + fmt.Sprintf(format, a...))
}
```

- [ ] **Step 3: 把 status + log 组装进主窗口**

修改 `ui/app.go` 的 `build` 方法：

```go
func (u *UI) build() {
	cfg, _ := config.Load("./config.json")
	u.cfg = cfg
	u.form = NewConfigForm(cfg)
	u.status = NewStatusPanel()
	u.logPanel = NewLogPanel()

	content := container.NewVBox(
		widget.NewLabelWithStyle("连接配置", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		u.form.Container(),
		widget.NewSeparator(),
		widget.NewLabelWithStyle("状态", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		u.status.Container(),
		widget.NewSeparator(),
		widget.NewLabelWithStyle("日志", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		u.logPanel.Container(),
	)
	u.window.SetContent(content)
}
```

并给 `UI` struct 加字段：

```go
type UI struct {
	app      fyne.App
	window   fyne.Window
	cfg      *config.Config
	form     *ConfigForm
	status   *StatusPanel
	logPanel *LogPanel
}
```

- [ ] **Step 4: 验证构建**

```bash
go build -o tether .
```

Expected: 成功

- [ ] **Step 5: 提交**

```bash
cd /root/windows-auto-stimulator
git add app/ui/
git commit -m "feat(ui): add status panel and log panel"
```

---

## Task 15: UI - 连接按钮接线

**Files:**
- Modify: `/root/windows-auto-stimulator/app/ui/app.go`
- Create: `/root/windows-auto-stimulator/app/ui/connector.go`

- [ ] **Step 1: 写 `ui/connector.go`**

这是把所有后端拼起来的"编排"层：

```go
package ui

import (
	"fmt"

	"tether/config"
	"tether/identity"
	"tether/sftpserver"
	"tether/sshclient"
)

type Connection struct {
	cfg         *config.Config
	admin       *identity.Identity
	sshClient   *sshclient.Client
	sftpSrv     *sftpserver.Server
	mountHandle *sshclient.MountHandle
	stopForward func()
}

// Connect 走完整流程: (可选) bootstrap -> 起 SFTP -> 反向转发 -> 远端挂载.
// password 在 Bootstrapped=false 时必填。
func Connect(cfg *config.Config, password string, log func(string)) (*Connection, error) {
	log("加载 admin 密钥...")
	admin, err := identity.GenerateOrLoad(cfg.AdminKeyPath)
	if err != nil {
		return nil, fmt.Errorf("load admin key: %w", err)
	}

	// 首次: 用密码 bootstrap
	if !cfg.Bootstrapped {
		if password == "" {
			return nil, fmt.Errorf("首次配置需要 VPS 密码")
		}
		log("用密码登录 VPS 做首次配置...")
		c, err := sshclient.DialPassword(cfg.VpsHost, cfg.VpsPort, cfg.VpsUser, password)
		if err != nil {
			return nil, fmt.Errorf("密码登录失败: %w", err)
		}
		log("安装 sshfs + 注册 admin 公钥...")
		if err := c.Bootstrap(admin.AuthorizedKey()); err != nil {
			c.Close()
			return nil, err
		}
		c.Close()
		cfg.Bootstrapped = true
		_ = config.Save("./config.json", cfg)
		log("首次配置完成")
	}

	// 常规: 用 admin 密钥登录
	log("用密钥登录 VPS...")
	admClient, err := sshclient.DialKey(cfg.VpsHost, cfg.VpsPort, cfg.VpsUser, admin)
	if err != nil {
		return nil, fmt.Errorf("密钥登录失败: %w", err)
	}

	// 起本地 SFTP server
	log("启动本地 SFTP 服务...")
	sftpHost, _ := identity.Ephemeral()
	sftpClient, _ := identity.Ephemeral()
	sftpSrv, err := sftpserver.Start(sftpserver.Config{
		RootDir:          cfg.SharePath,
		HostIdentity:     sftpHost,
		AllowedClientKey: sftpClient.PublicKey(),
	})
	if err != nil {
		admClient.Close()
		return nil, fmt.Errorf("SFTP 启动失败: %w", err)
	}

	// 反向转发 VPS:2222 -> localhost:sftpPort
	log(fmt.Sprintf("建立反向隧道 VPS:%d -> 本地:%d", cfg.RemoteTunnelPort, sftpSrv.Port()))
	remoteAddr := fmt.Sprintf("localhost:%d", cfg.RemoteTunnelPort)
	localAddr := fmt.Sprintf("127.0.0.1:%d", sftpSrv.Port())
	stop, err := admClient.ReverseForward(remoteAddr, localAddr)
	if err != nil {
		sftpSrv.Close()
		admClient.Close()
		return nil, fmt.Errorf("反向转发失败: %w", err)
	}

	// 远端挂载
	log("在 VPS 上执行 sshfs 挂载...")
	handle, err := admClient.Mount(sshclient.MountParams{
		WinShareUser:      "winshare",
		RemoteTunnelPort:  cfg.RemoteTunnelPort,
		RemoteMountPoint:  cfg.RemoteMountPoint,
		SFTPPrivateKeyPEM: sftpClient.PrivatePEM(),
	})
	if err != nil {
		stop()
		sftpSrv.Close()
		admClient.Close()
		return nil, err
	}
	log("挂载成功")
	return &Connection{
		cfg:         cfg,
		admin:       admin,
		sshClient:   admClient,
		sftpSrv:     sftpSrv,
		mountHandle: handle,
		stopForward: stop,
	}, nil
}

// Disconnect 清理所有资源
func (c *Connection) Disconnect(log func(string)) {
	if c == nil {
		return
	}
	if c.sshClient != nil && c.mountHandle != nil {
		log("卸载远端...")
		if err := c.sshClient.Unmount(c.mountHandle); err != nil {
			log("unmount err: " + err.Error())
		}
	}
	if c.stopForward != nil {
		log("关闭反向隧道...")
		c.stopForward()
	}
	if c.sftpSrv != nil {
		log("关闭本地 SFTP 服务...")
		_ = c.sftpSrv.Close()
	}
	if c.sshClient != nil {
		_ = c.sshClient.Close()
	}
	log("已全部清理")
}

func (c *Connection) AdminKeyPath() string { return c.cfg.AdminKeyPath }
```

- [ ] **Step 2: 在 `ui/app.go` 加按钮和事件**

给 `UI` 加字段：

```go
type UI struct {
	// ... 原有字段
	connectBtn    *widget.Button
	disconnectBtn *widget.Button
	conn          *Connection
}
```

修改 `build` 方法，在 form 下面加按钮行，在 status 上面：

```go
u.connectBtn = widget.NewButton("🔌 连接", u.onConnect)
u.disconnectBtn = widget.NewButton("⏹ 断开", u.onDisconnect)
u.disconnectBtn.Disable()

buttons := container.NewHBox(u.connectBtn, u.disconnectBtn)

content := container.NewVBox(
	widget.NewLabelWithStyle("连接配置", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
	u.form.Container(),
	buttons,
	widget.NewSeparator(),
	// ... rest unchanged
)
```

添加事件处理方法：

```go
func (u *UI) onConnect() {
	u.connectBtn.Disable()
	u.status.Set(StateConnecting)
	password := u.form.Snapshot()
	go func() {
		conn, err := Connect(u.cfg, password, u.logPanel.Append)
		if err != nil {
			u.logPanel.Error("%v", err)
			u.status.Set(StateDisconnected)
			u.connectBtn.Enable()
			return
		}
		u.conn = conn
		u.form.ClearPassword()
		_ = config.Save("./config.json", u.cfg)
		u.status.Set(StateConnected)
		u.disconnectBtn.Enable()
	}()
}

func (u *UI) onDisconnect() {
	u.disconnectBtn.Disable()
	go func() {
		u.conn.Disconnect(u.logPanel.Append)
		u.conn = nil
		u.status.Set(StateDisconnected)
		u.connectBtn.Enable()
	}()
}
```

同时在主窗口关闭时做清理：

```go
// 在 build() 末尾追加
u.window.SetCloseIntercept(func() {
	if u.conn != nil {
		u.conn.Disconnect(u.logPanel.Append)
	}
	u.window.Close()
})
```

- [ ] **Step 3: 验证构建**

```bash
go build -o tether .
```

Expected: 成功

- [ ] **Step 4: 提交**

```bash
cd /root/windows-auto-stimulator
git add app/ui/
git commit -m "feat(ui): wire connect/disconnect buttons to backend"
```

---

## Task 16: UI - 启动 Claude 按钮

**Files:**
- Modify: `/root/windows-auto-stimulator/app/ui/app.go`

- [ ] **Step 1: 加按钮**

给 `UI` 加字段：

```go
claudeBtn *widget.Button
```

在 `build` 中，status 之后、log 之前加一行：

```go
u.claudeBtn = widget.NewButton("🚀 启动 Claude Code", u.onLaunchClaude)
u.claudeBtn.Disable()

content := container.NewVBox(
	// ... 前半部分
	widget.NewSeparator(),
	widget.NewLabelWithStyle("状态", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
	u.status.Container(),
	u.claudeBtn,           // ← 新增
	widget.NewSeparator(),
	widget.NewLabelWithStyle("日志", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
	u.logPanel.Container(),
)
```

- [ ] **Step 2: 在连接成功 / 断开时切换按钮状态**

在 `onConnect` 成功分支添加：

```go
u.claudeBtn.Enable()
```

在 `onDisconnect` 成功分支添加：

```go
u.claudeBtn.Disable()
```

- [ ] **Step 3: 写事件处理**

```go
func (u *UI) onLaunchClaude() {
	if u.conn == nil {
		return
	}
	err := claude.Launch(claude.LaunchParams{
		AdminKeyPath:     u.cfg.AdminKeyPath,
		VpsHost:          u.cfg.VpsHost,
		VpsUser:          u.cfg.VpsUser,
		VpsPort:          u.cfg.VpsPort,
		RemoteMountPoint: u.cfg.RemoteMountPoint,
	})
	if err != nil {
		u.logPanel.Error("启动 Claude 失败: %v", err)
		return
	}
	u.logPanel.Append("已启动新的 Claude 会话窗口")
}
```

加 import：

```go
import "tether/claude"
```

- [ ] **Step 4: 验证构建**

```bash
go build -o tether .
```

Expected: 成功

- [ ] **Step 5: 提交**

```bash
cd /root/windows-auto-stimulator
git add app/ui/
git commit -m "feat(ui): add Launch Claude button"
```

---

## Task 17: 窗口关闭清理保证

**Files:**
- Modify: `/root/windows-auto-stimulator/app/ui/app.go`

（Task 15 已加了 `SetCloseIntercept`；此任务确认并增强：清理完成后再真正关闭窗口，避免主进程先退导致 ssh 连接被粗暴 kill。）

- [ ] **Step 1: 改成同步清理**

把 `SetCloseIntercept` 的实现改成：

```go
u.window.SetCloseIntercept(func() {
	if u.conn != nil {
		u.logPanel.Append("窗口关闭, 清理中...")
		u.conn.Disconnect(u.logPanel.Append)
		u.conn = nil
	}
	u.window.Close()
})
```

- [ ] **Step 2: 验证构建**

```bash
go build -o tether .
```

- [ ] **Step 3: 提交**

```bash
cd /root/windows-auto-stimulator
git add app/ui/
git commit -m "fix(ui): ensure cleanup before window close"
```

---

## Task 18: 交叉编译到 Windows

**Files:** 无新代码。

- [ ] **Step 1: 交叉编译**

```bash
cd /root/windows-auto-stimulator/app
GOOS=windows GOARCH=amd64 CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc \
  go build -ldflags "-s -w -H=windowsgui" -o tether.exe .
ls -lh tether.exe
file tether.exe
```

Expected:
- 文件 15–30 MB
- `file` 输出 `PE32+ executable (GUI) x86-64, for MS Windows`

- [ ] **Step 2: 验证 Linux 端 go vet 和 test 依然绿**

```bash
go vet ./...
go test ./config/... ./identity/... ./sftpserver/... ./sshclient/...
```

Expected: 全 PASS

- [ ] **Step 3: 提交**

```bash
cd /root/windows-auto-stimulator
git add app/
git commit -m "chore: build Windows tether.exe"
```

（tether.exe 已在 .gitignore 中，不会被提交本身）

---

## Task 19: 端到端冒烟测试

**Files:** 无代码修改。本任务是人工验证。

**环境**：
- VPS：本机 (172.236.229.37)
- Windows：用户 Yu 的 Windows 机器

- [ ] **Step 1: 准备 VPS 干净态**

```bash
# 清掉之前 PowerShell 方案留下的痕迹
> ~/.ssh/authorized_keys
rm -f ~/.windows-auto-stim.env
rm -rf ~/local-code
```

- [ ] **Step 2: 分发 exe**

把 `/root/windows-auto-stimulator/app/tether.exe` 复制到 Windows (SCP / 网盘 / 其它方式)，放到任意空目录，比如 `D:\tether-test\`。

- [ ] **Step 3: 首次运行**

双击 `tether.exe`：
- Windows 无需管理员权限
- 填写 VPS 地址、VPS 密码、共享目录（例如 `D:\tether-test`）
- 点"连接"
- 期望日志依次输出：
  - 加载 admin 密钥
  - 用密码登录 VPS 做首次配置
  - 安装 sshfs + 注册 admin 公钥
  - 首次配置完成
  - 用密钥登录 VPS
  - 启动本地 SFTP 服务
  - 建立反向隧道
  - 在 VPS 上执行 sshfs 挂载
  - 挂载成功
- 状态变"● 已连接"
- "启动 Claude Code" 按钮亮起

- [ ] **Step 4: 在 VPS 上验证挂载**

```bash
ls ~/local-code
touch ~/local-code/from-vps.txt
```

Expected: `from-vps.txt` 在 Windows 的 `D:\tether-test\` 里立刻出现

- [ ] **Step 5: 点"启动 Claude Code"按钮**

Expected:
- 新 PowerShell 窗口打开
- 自动 SSH 到 VPS 并进入 `~/local-code` 目录
- `claude` 启动

在 Claude 里创建一个文件：
```
请在当前目录创建 test.md, 内容 "hello from claude"
```

Expected: `D:\tether-test\test.md` 出现且内容正确。

- [ ] **Step 6: 点"断开"按钮**

Expected 日志：
- 卸载远端...
- 关闭反向隧道...
- 关闭本地 SFTP 服务...
- 已全部清理

状态变回"● 未连接"。

在 VPS：
```bash
ls ~/local-code
ls /tmp/.tether-*
```

Expected:
- `~/local-code` 空（未挂载）
- `/tmp/.tether-*` 不存在（临时私钥已删）

- [ ] **Step 7: 再次运行 (免密)**

重新双击 `tether.exe`，点"连接"。

Expected:
- 不再询问密码
- 秒级连接成功

- [ ] **Step 8: 关闭窗口清理测试**

直接点 exe 窗口的 X。

Expected:
- 窗口短暂保持（执行清理）
- VPS 上 `ls ~/local-code` 为空；`/tmp/.tether-*` 不存在

- [ ] **Step 9: 记录结果**

在 plan 文档末尾追加测试记录（成功/失败的现象）。

- [ ] **Step 10: 提交最终修复**（如测试中发现小问题）

```bash
cd /root/windows-auto-stimulator
git add .
git commit -m "fix: address E2E smoke test issues"
```

---

## 风险与回滚

- **构建失败**：Task 0 的工具链准备可能因 VPS 缺包卡住 → 逐包排查 `apt install` 报错，必要时换 `fyne-cross` (Docker)。
- **Fyne GUI 在 Windows 崩**：最可能是 OpenGL / DWM 问题 → 查 `tether.exe` 启动时 stderr（可以临时用 `-ldflags "-H=windows"` 保留控制台）。
- **sshfs 挂载失败**：日志会展示 VPS 返回的 stderr → 最常见是 fuse 设备权限问题，VPS 上 `modprobe fuse` 或检查 `/dev/fuse` 权限。
- **反向转发被 sshd 拒**：`grep AllowTcpForwarding /etc/ssh/sshd_config`，默认应为 yes。
- **回滚策略**：整个项目独立于旧的 `tunnel.bat` / `mount.sh`，完成前旧脚本保持可用；完成后可以手动删除旧脚本（本计划不做）。

---

## 完成标准

1. `tether.exe` 能在 Windows 上双击运行
2. 首次配置只需填 VPS 信息 + 密码一次
3. 挂载成功后 Windows 本地文件和 VPS `~/local-code` 完全同步
4. 点"启动 Claude Code"能在新 PowerShell 窗口里用 Claude 编辑 Windows 本地文件
5. 断开 / 关窗自动清理挂载和临时文件
6. Linux 端 `go test ./...` 全部通过
