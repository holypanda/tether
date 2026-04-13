# tether

让 VPS 上的程序直接读写 Windows 本地的项目文件——一个 exe，零配置，零权限。

**Status**：v1.0.0，初步交付。已通过完整 E2E 冒烟测试。

## 是什么

一个 Windows GUI 工具（单文件 `.exe`，约 23 MB），解决这个问题：

> 我用 Claude Code 在 VPS 上跑，但代码放在 Windows 本地。我不想同步、不想 scp、不想 git push/pull 来回倒。我希望 Claude 看到的 `~/local-code/*` **就是**我 Windows 上的 `D:\my-project\*`，改动实时双向生效。

tether 在 Windows 上开一个嵌入式 SFTP 服务器（只监听 `127.0.0.1`），通过反向 SSH 隧道把这个本地服务暴露给 VPS，VPS 上的 `sshfs` 反向连回来。实际效果：

```
你的 Windows                              VPS
┌──────────────────┐                 ┌─────────────────┐
│  D:\my-project\  │                 │  ~/local-code/  │
│   ├── main.py    │    ← 网络 ←     │   ├── main.py   │  ← sshfs 虚拟目录
│   ├── README.md  │                 │   ├── README.md │    (没有真实文件)
│   └── ...        │                 │   └── ...       │
│                  │                 │                 │
│  ┌────────────┐  │   反向 SSH 隧道 │  ┌───────────┐  │
│  │ 嵌入式     │◀─┼──────────────── │  │  sshfs    │  │
│  │ SFTP 服务  │  │                 │  └───────────┘  │
│  └────────────┘  │                 │        ▲        │
└──────────────────┘                 │        │        │
                                     │  ┌─────┴──────┐ │
                                     │  │ claude TUI │ │  ← 跑在 VPS
                                     │  └────────────┘ │
                                     └─────────────────┘
```

Claude 的**进程**跑在 VPS 上（用 VPS 的 CPU、内存、网络），但它**操作的文件**在 Windows 上——每一次读写都通过 sshfs → 反向隧道 → exe 的 SFTP 服务，最终落到 Windows 本地磁盘。

## 核心特点

- **真正单文件**：一个 `tether.exe`，拷到任何 Windows 双击就跑
- **零管理员权限**：不装 OpenSSH Server、不改防火墙、不动系统服务
- **首次仅一次密码**：之后全程 Ed25519 密钥免密登录
- **安全**：VPS 回连 Windows 的私钥仅生存于当前连接内，断开即销毁；SFTP 服务严格 chroot 到共享目录
- **可视状态**：连接状态、运行时长、实时日志
- **集成 SSH 终端**：一键开 cmd 窗口直达 VPS `~/local-code`，可跑 `claude`、`git`、任何命令

## 快速开始

### 下载

从 release 获取 `tether.exe`（或自行交叉编译，见下）。

### 首次运行

1. 把 `tether.exe` 放到任意文件夹（会在旁边生成 `tether.json` 和 `keys/` 目录）
2. 双击运行
3. 填写表单：
   - **VPS 地址**：`1.2.3.4`
   - **VPS 用户**：`root`
   - **VPS 端口**：`22`
   - **VPS 密码**：（仅首次需要，不会存盘）
   - **共享目录**：Windows 本地项目路径，可点 📁 按钮浏览
   - **远端挂载**：VPS 上想挂在哪里，默认 `~/local-code`
4. 点 **🔌 连接**

首次连接会做这些事（过程可在日志面板实时看到）：

1. 生成本机 Ed25519 admin 密钥（保存到 `./keys/admin_ed25519`）
2. 用密码 SSH 到 VPS
3. 在 VPS 上装 `sshfs`（如尚未安装）
4. 把 admin 公钥追加到 VPS 的 `~/.ssh/authorized_keys`
5. 启动本地 SFTP 服务器（`127.0.0.1:随机端口`）
6. 用密钥建立反向隧道（`VPS:2222 → 本机:SFTP端口`）
7. 在 VPS 上跑 `sshfs` 挂载

完成后状态变 **● 已连接**，密码字段清空。

### 日常使用

以后每次双击 `tether.exe`：

1. 表单自动从 `tether.json` 加载
2. 点 **🔌 连接**——秒级完成，无需密码
3. 点 **🔗 打开 VPS SSH 窗口**——开一个新的 cmd 窗口，自动 SSH 到 VPS 并 cd 到挂载目录；可立即 `claude`
4. 用完点 **⏹ 断开** 或直接关窗口——自动 unmount + 清理临时文件

### VPS 上验证

```bash
ls ~/local-code          # 应看到你 Windows 项目的所有文件
touch ~/local-code/test  # 立即出现在 Windows 本地
```

## 使用建议（重要）

tether 让文件"出现"在 VPS 上，但**运行环境**仍然是 VPS 的 Linux。不是在 Windows 里开发。两边的关系如下：

| | 位置 |
|---|---|
| 源代码字节 | Windows 本地磁盘 ✅ |
| Claude 进程 | VPS Linux |
| `python xxx.py` / `npm run dev` | VPS Linux |
| 依赖包（pip/npm/go mod） | VPS Linux |
| 编译产物 | VPS Linux（ELF） |

**核心原则**：把挂载目录当成"源码" only。环境 / 依赖 / 构建产物在**各自一侧**独立管理。

### 推荐的工作流

#### 1. Manifest 驱动的依赖管理

挂载目录里只放源码 + manifest。不要放 `venv/` / `node_modules/` / `target/` / `__pycache__/`（慢 + 里面有平台相关二进制）。`.gitignore` 示例：

```
venv/
.venv/
node_modules/
target/
dist/
__pycache__/
*.pyc
.pytest_cache/
```

Claude 在 VPS 跑 `pip install X` / `npm install X` 之后，**必须同时更新** `requirements.txt` / `package.json`。然后你 Windows 侧跑一次对应的 install 命令，两边依赖对齐。

#### 2. CLAUDE.md 约束 Claude 的行为

项目根放一个 `CLAUDE.md`，告诉 Claude 这是个 tether 环境：

```markdown
# Environment constraints

This project is edited via tether (sshfs mount): source files live on Windows,
but Python/Node/Go runtime is on the Linux VPS. Source runs on VPS for testing,
but must ultimately work on Windows too.

**Dependency rule (strict):**
- NEVER `pip install` / `npm install` without also adding the package to
  requirements.txt / package.json in the same turn.
- After adding a dependency, tell me: "⚠️ new dep added, run `pip install -r
  requirements.txt` (or equivalent) on Windows before local testing."
- NEVER create or commit venv/, node_modules/, or build output.

**Shell / path rule:**
- Code must be cross-platform if it ever needs to run on Windows.
- Avoid hardcoded `/tmp`, `/usr/bin/...`, POSIX-only syscalls.
- Use `os.path.join`, `pathlib`, or equivalent — never `"/"`.
```

Claude Code 启动时会自动读取此文件，严格遵守规则。

#### 3. Windows 侧的 "sync" 一键脚本

项目根放一个 `sync-deps.ps1`：

```powershell
# 跑 Claude 后同步 VPS 侧新装的依赖到 Windows
if (Test-Path requirements.txt) { python -m pip install -r requirements.txt }
if (Test-Path package.json)     { npm ci }
if (Test-Path go.mod)           { go mod download }
Write-Host "Dependencies synced. You can now run locally on Windows."
```

看到 Claude 说 "⚠️ new dep added" 就双击一下，两边环境立刻对齐。

#### 4. git 操作在 Windows 本地跑

不要在 `~/local-code`（sshfs 挂载点）上跑 `git commit/pull/push`——`.git` 下几百上千个小对象文件通过反向隧道走非常慢，还可能 lock / 权限问题。在 Windows 侧用 VSCode 内置终端或 Git Bash 跑 git。

#### 5. 最终测试必须在 Windows 本地跑

tether 里的 Claude 测试通过 ≠ Windows 本地能跑。最后一步一定要 `cd D:\project && <run command>` 在本地亲自试一遍。

### tether 什么时候特别香 vs 用处有限

**特别香**：
- 纯前端项目（HTML/CSS/JS/TS）——文件就是全部，Windows 浏览器直接能看
- 纯 Markdown / 文档 / 配置文件
- 目标部署到 Linux 的项目（VPS 环境 ≈ 生产环境）
- Claude 做重构 / 批量代码修改——只动文件不跑代码

**用处有限**：
- Windows-only 应用（依赖 Win32 API、.NET 等）——仍然可以用 Claude 编辑，但测试必须离开 tether
- 依赖大量本地二进制包的项目（`sharp`、`esbuild`、`pandas` 的部分 wheel）——两边装两遍是必须的

## 架构

### 模块

```
app/
├── main.go                      # 入口 → ui.New().Run()
├── config/
│   ├── config.go                # Config 结构体 + JSON 读写
│   └── config_test.go
├── identity/
│   ├── identity.go              # Ed25519 密钥生成/加载 (OpenSSH PEM 格式)
│   ├── identity_test.go
│   ├── permfix_windows.go       # NTFS ACL 锁死 (icacls)
│   └── permfix_other.go         # POSIX no-op
├── sftpserver/
│   ├── server.go                # 内嵌 SFTP 服务 + chroot handlers
│   └── server_test.go
├── sshclient/
│   ├── client.go                # DialPassword/DialKey + ReverseForward
│   ├── client_test.go
│   ├── remote.go                # Run / RunScript (base64 封装)
│   ├── bootstrap.go              # 首次 VPS 初始化
│   └── mount.go                 # sshfs mount/unmount 远程脚本
├── claude/
│   ├── launcher_windows.go      # 新开 cmd 窗口 ssh 到 VPS
│   └── launcher_other.go        # 非 Windows 下 no-op
└── ui/
    ├── app.go                   # Fyne 主窗口装配 + 按钮事件
    ├── form.go                  # 配置表单 + 文件夹选择器
    ├── status.go                # 连接指示灯 + 运行时长
    ├── log.go                   # 滚动日志面板
    └── connector.go             # 全流程编排 (Connect / Disconnect)
```

### 每个包一句话

| 包 | 职责 |
|---|---|
| `config` | `tether.json` 读写，失败时返回 defaults 保证不返回 nil |
| `identity` | Ed25519 密钥生成/加载，OpenSSH PEM 格式，Windows 上自动锁 ACL |
| `sftpserver` | 内嵌 SFTP 服务器，单 pubkey 白名单，chroot 到 RootDir 防逃逸 |
| `sshclient` | SSH 客户端封装：拨号、命令执行、反向转发、VPS 初始化、sshfs mount |
| `claude` | 平台相关的"开 SSH 终端"启动器，Windows 通过临时 .cmd 文件 |
| `ui` | Fyne 界面 + 把其它所有包编排起来的 connector |

### 运行时流程

```
用户点"连接"
    │
    ├─ 加载/生成 admin 密钥
    │    └─ Windows: icacls 锁 ACL
    │
    ├─ 首次: 用密码 SSH → Bootstrap(装 sshfs + 注册公钥) → 保存 Bootstrapped=true
    │
    ├─ 用密钥 SSH → admClient
    │
    ├─ probe $HOME → 解析远端挂载路径的 ~
    │
    ├─ 生成两对 ephemeral 密钥 (SFTP host + client)
    │
    ├─ 启动本地 SFTP 服务器 (127.0.0.1:随机端口, 白名单 client pubkey)
    │
    ├─ ReverseForward: VPS:2222 → 本机:SFTP 端口
    │
    └─ Mount: 把 SFTP 私钥临时写到 /tmp/.tether-xxx → 跑 sshfs → 立即 rm 私钥
```

关闭时按相反顺序拆。

### 安全模型

- **密码零持久化**：首次配置用完立刻从内存和 UI 清除
- **admin 密钥**：明文存本地 `./keys/admin_ed25519`（OpenSSH 格式），Windows 上强制 NTFS ACL 只有当前用户可读
- **SFTP 访问密钥**：每次连接新生成的 ephemeral 私钥通过 admin SSH 通道推到 VPS `/tmp/.tether-<rand>`（`umask 077`），sshfs 启动后立即 `rm -f` 清掉；失败时 `trap EXIT` 兜底清理
- **SFTP 服务只监听 `127.0.0.1`**：外网绝对无法触达
- **反向隧道只绑定 VPS localhost**：默认 `GatewayPorts no`，VPS 外部也无法访问
- **Shell 注入防护**：所有传进 sshfs 脚本的路径/用户名用 `'...'` 单引号 quote，`WinShareUser` 强制白名单 `[a-zA-Z0-9_-]+`
- **SFTP 路径逃逸防护**：自定义 `sftp.Handlers` 把所有请求路径 `filepath.Clean + Join + Rel` 后校验必须在 `RootDir` 子树内，`Symlink`/`Link` 直接返回 `ErrPermission`

## 配置文件

`tether.json`（与 exe 同目录，自动生成）：

```json
{
  "vpsHost": "1.2.3.4",
  "vpsUser": "root",
  "vpsPort": 22,
  "sharePath": "D:\\my-project",
  "remoteMountPoint": "~/local-code",
  "remoteTunnelPort": 2222,
  "adminKeyPath": "./keys/admin_ed25519",
  "bootstrapped": true
}
```

**不包含密码**——密码只在首次 bootstrap 时用一次。`bootstrapped: true` 之后 Load 时这个字段让 UI 跳过密码提示。

## 构建

### 运行时需求

- Windows 10 或更新（需要 OpenGL 支持，远程桌面/某些云桌面可能缺）
- VPS：Linux（Debian/Ubuntu/CentOS），`sshd` 允许 `AllowTcpForwarding`（默认允许），`fuse` 内核模块可用
- VPS：首次连接时能连到互联网（装 sshfs）

### 构建环境（Linux 交叉编译到 Windows）

```bash
# Go 1.22+
curl -LO https://go.dev/dl/go1.22.5.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.22.5.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin

# mingw-w64 交叉编译器 + Fyne 所需 X11/GL headers
sudo apt-get install -y gcc-mingw-w64-x86-64 pkg-config libgl1-mesa-dev xorg-dev
```

### 构建

```bash
cd app

# Linux 本地测试 (GUI 需要显示设备)
go build -o tether .

# Windows 交叉编译 (release, 无控制台窗口)
GOOS=windows GOARCH=amd64 CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc \
  go build -ldflags "-s -w -H=windowsgui" -o tether.exe .

# Windows debug 版本 (有控制台, 能看到 panic 堆栈)
GOOS=windows GOARCH=amd64 CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc \
  go build -o tether-debug.exe .
```

### 测试

```bash
cd app
go test ./... -race
go vet ./...
```

核心包（`config`、`identity`、`sftpserver`、`sshclient`）都有单元测试，包含 race detector 通过。UI / Claude 启动器这种平台或 GUI 相关的部分由端到端手动验证。

## 常见问题

### 双击 exe 没反应 / 闪退

用 debug 版本在 PowerShell 里跑：

```powershell
.\tether-debug.exe
```

panic 堆栈会直接打印到终端。常见原因：
- **OpenGL 初始化失败**：通常发生在云桌面 / 旧 RDP 会话 / 无 3D 加速的虚拟机，本机真实桌面正常
- **config.json 格式不对**（v1.0.0 前的 release 才有）：已修，Load 永不返回 nil，且配置文件改名为 `tether.json`

### 连接时一直提示输密码

Windows OpenSSH 严格检查私钥文件 ACL——如果文件对当前用户以外的任何身份可读就**静默忽略**落回密码提示。tether 每次加载 / 生成密钥时都会自动 `icacls` 锁死 ACL。如果还是不行，手动修：

```powershell
icacls .\keys\admin_ed25519 /inheritance:r /grant:r "$env:USERNAME:F"
```

### 连接按钮附近闪过一个 cmd 黑窗口

v1.0.0 之前的版本会在每次 `icacls` 时闪一下控制台窗口（GUI 程序 spawn console 子进程的默认行为）。v1.0.0 之后加了 `CREATE_NO_WINDOW` 标志完全隐藏。确保你用的是最新 exe。

### 挂载路径变成 `/root/~/local-code`（字面的 ~）

v1.0.0 之前 Mount 脚本对路径做单引号 quote 时阻止了 bash 的 `~` 展开。v1.0.0 之后会在连接时先 `printf '%s' "$HOME"` 拿到真实家目录再做字符串替换。

### 打开 SSH 窗口时报 `'&&' is not a valid statement separator`

Win10 默认的 PowerShell 5.1 不支持 `&&`。v1.0.0 之后用临时 `.cmd` 批处理绕开 PowerShell，不再有这个问题。

### 大项目 grep / find 很慢

sshfs 每次文件元数据查询都走反向隧道，大目录（`node_modules`、`.git`、`target`）会明显慢。建议：
- 把这些目录放进 `.gitignore` 和 `.claudeignore`
- **别在挂载点跑 git**——用 Windows 本地 Git Bash / VSCode 终端
- **别在挂载点跑 `npm install` / `pip install`**——见上文"使用建议"
- Claude 只操作源文件，体验完全可接受

### Ctrl+C 关闭 exe 后 VPS 上残留 `/tmp/.tether-*`

正常退出（点断开或关窗口）会清理。**强杀进程**（任务管理器强行结束）会留下临时私钥。下次连接前手动清：

```bash
ssh root@vps 'rm -f /tmp/.tether-*'
```

即使泄漏风险也有限：临时私钥只能访问**这台 Windows 当前共享目录**，且每次连接都会换一对新密钥。

## 路线图 / 未解决

下一期可以考虑加的功能（按大致优先级）：

- [ ] **依赖同步**：文件监听 `SharePath/` 的 manifest 文件，检测到改动在 UI 亮一个徽章，一键跑 `pip install` / `npm ci` 等同步 Windows 侧——把上面"使用建议"里手动搞的 `sync-deps.ps1` 吸收进 tether 本体
- [ ] 首次 bootstrap 失败时友好诊断（检测 sshd `AllowTcpForwarding`、`fuse` 内核模块、sshfs 装包失败的原因）
- [ ] 自动重连（网络抖动后 sshfs 僵尸恢复）
- [ ] 多 VPS profile 切换
- [ ] 托盘图标最小化
- [ ] Windows DPAPI 加密保存 admin 私钥

## 版本历程

**v1.0.0**（2026-04-13）— 初步交付
- 所有 E2E 场景通过
- 34 个 commit
- 项目从 `stim-link` 重命名为 `tether`
- 6 个 Go 包（config/identity/sftpserver/sshclient/claude/ui），~1900 行
- 核心包带 race detector 单元测试全绿

从 PoC PowerShell 脚本到 Go + Fyne 单 exe 的完整历程，全流程走 spec → plan → TDD → 代码审查 → 修复的节奏，踩过 10 多个坑（Ed25519 PEM 格式、sftp chroot 逃逸、ReverseForward stop 双调用、shell 注入、UI 并发 race、config 冲突、PowerShell `&&` 不兼容、NTFS ACL 问题、bash 单引号 `~` 展开、icacls 闪窗…）。

详见 `docs/superpowers/specs/` 的设计文档、`docs/superpowers/plans/` 的按任务分解计划、以及：

```bash
git log --oneline
```

## 目录结构总览

```
tether/
├── README.md                   ← 本文件
├── .gitignore
├── app/                        ← Go 源代码
│   ├── go.mod
│   ├── go.sum
│   ├── main.go
│   ├── config/
│   ├── identity/
│   ├── sftpserver/
│   ├── sshclient/
│   ├── claude/
│   └── ui/
├── docs/
│   └── superpowers/
│       ├── specs/
│       │   └── 2026-04-11-windows-vps-mount-gui-design.md
│       └── plans/
│           └── 2026-04-11-windows-vps-mount-gui.md
├── vps/                        ← 遗留的纯 bash 方案 (备用)
│   ├── mount.sh
│   └── unmount.sh
└── windows/                    ← 遗留的纯 PowerShell 方案 (备用)
    ├── tunnel.bat
    └── tunnel.ps1
```

`vps/` 和 `windows/` 目录是项目最早用纯脚本实现时的产物，保留作为降级方案——如果某天 Go exe 出了奇怪问题，这些脚本还能直接用（注意：这些脚本里用的老名字还是 `stim-link` / `windows-auto-stim.env`，没改过来）。

## License

TBD（内部工具，目前没选）。

## 致谢

- [Fyne](https://fyne.io/) — 纯 Go 的 GUI 工具包，让单文件交叉编译成为可能
- [pkg/sftp](https://github.com/pkg/sftp) — Go 的 SFTP 服务端 / 客户端实现
- [golang.org/x/crypto/ssh](https://pkg.go.dev/golang.org/x/crypto/ssh) — 原生 Go SSH 客户端 / 服务端，彻底摆脱对系统 `ssh.exe` 的依赖
