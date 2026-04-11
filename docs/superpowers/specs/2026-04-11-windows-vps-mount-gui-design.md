# Windows ↔ VPS 开发挂载工具 设计文档

- 日期：2026-04-11
- 状态：设计中
- 作者：Yu (brainstorm with Claude)

## 背景与目标

### 痛点
当前方案需要在两台机器上各跑一个脚本：
- Windows：`tunnel.bat`（PowerShell，建反向 SSH 隧道）
- VPS：`mount.sh`（读配置、挂载 sshfs）

并且依赖 Windows 的 OpenSSH Server，踩过一系列坑：管理员权限、ACL、`administrators_authorized_keys`、防火墙规则、`$HOME` 变量展开 bug 等。

### 目标
打造一个**单文件 Windows 可执行程序**，实现：
1. **零安装**：拷到任何 Windows 机器双击即用，不需要 Go/PowerShell 环境、不需要管理员权限
2. **一键连接**：简单 GUI 填写 VPS 信息，点"连接"即完成反向隧道 + 远程 sshfs 挂载
3. **零 VPS 副作用**：VPS 上除一个临时文件和 `authorized_keys` 一行公钥外，不留任何脚本、服务、配置
4. **集成开发入口**：挂载成功后提供"启动 Claude Code"按钮，新开终端窗口直接进 VPS 跑 `claude`，编辑的是 Windows 本地文件

### 非目标
- 不做 Windows OpenSSH Server 相关的任何安装/配置
- 不做 git/npm/pip 加速（这些命令用户自行在 Windows 本地执行）
- 不做跨平台 GUI（只面向 Windows；macOS/Linux 用户可以直接用 sshfs）
- 不做多 VPS 同时挂载（一个 exe 一次连一个 VPS）

## 关键洞察

让 .exe **自身内嵌一个 SFTP 服务器**（用 Go 的 `crypto/ssh` + `pkg/sftp`），只监听 `127.0.0.1`，通过反向 SSH 隧道把这个本地 SFTP 服务暴露给 VPS，VPS 上的 sshfs 反向连过来。

这样做彻底规避了"必须在 Windows 上装 OpenSSH Server"的所有问题：不需要管理员、不需要防火墙规则、不需要管理 authorized_keys。

## 总体架构

```
Windows (.exe)                                 VPS
┌─────────────────────────┐              ┌──────────────────────┐
│ Fyne GUI                │              │                      │
│ ┌─────────────────────┐ │              │   ~/local-code/      │
│ │ 配置表单            │ │              │   ├── main.py  (虚)  │
│ │ 状态面板            │ │              │   └── ...      (虚)  │
│ │ 启动 Claude 按钮    │ │              │        ▲             │
│ └─────────────────────┘ │              │        │ sshfs       │
│                         │              │        │             │
│ ┌─────────────────────┐ │   反向隧道   │  ┌─────┴──────┐     │
│ │ 嵌入式 SFTP Server  │◀┼──────────────┼──│ sshfs 进程 │     │
│ │ 127.0.0.1:<随机>    │ │  2222 → ???  │  └────────────┘     │
│ │ 服务于: 共享目录    │ │              │                      │
│ └─────────────────────┘ │              │  ┌─────────────┐    │
│                         │              │  │ claude (TUI)│    │
│ ┌─────────────────────┐ │   admin SSH  │  └─────────────┘    │
│ │ SSH 客户端 (admin)  │◀┼──────────────┤        ▲             │
│ │ 用于远程命令/转发   │ │              │        │             │
│ └─────────────────────┘ │              │   用户从 Win 新开    │
│                         │              │   PowerShell ssh 进  │
└─────────────────────────┘              └──────────────────────┘
```

## 组件清单

### 1. GUI 层（Fyne v2）
- 配置表单：VPS 地址、用户、端口、密码（仅首次）、共享目录、远端挂载点
- 保存/连接按钮
- 状态面板：连接状态、运行时长、隧道/挂载指示灯
- 启动 Claude Code 按钮（挂载成功后启用）
- 滚动日志区（只显示关键事件，不做详尽输出）
- 断开按钮

### 2. 配置存储
- `config.json`，与 exe 同目录
- 字段：
  ```json
  {
    "vpsHost": "172.236.229.37",
    "vpsUser": "root",
    "vpsPort": 22,
    "sharePath": "D:\\stimulator-automator",
    "remoteMountPoint": "~/local-code",
    "remoteTunnelPort": 2222,
    "adminKeyPath": "./keys/admin_ed25519",
    "bootstrapped": true
  }
  ```
- **不存密码**；密码仅用于首次 bootstrap

### 3. 密钥管理
两对 Ed25519 密钥：
- **admin key**（持久化）：
  - Windows → VPS 的长期免密登录
  - 生成路径：`./keys/admin_ed25519`（与 exe 同目录）
  - 对应公钥首次 bootstrap 时写入 VPS 的 `~/.ssh/authorized_keys`
- **sftp access key**（临时，每次连接新生成）：
  - 内嵌 SFTP Server 只接受此公钥
  - 对应私钥通过 admin SSH 通道推到 VPS `/tmp/.wasm-<随机16字节>`（权限 600）
  - 断开时通过 admin SSH 通道删除

### 4. 嵌入式 SFTP Server
- 依赖：`golang.org/x/crypto/ssh` + `github.com/pkg/sftp`
- 监听：`127.0.0.1:0`（随机空闲端口，避免冲突）
- 认证：只接受本次连接的 sftp access pubkey
- 文件系统根：用户配置的 `sharePath`
- 权限：读写

### 5. SSH 客户端 (admin)
- 一个常驻的 SSH 连接对象，生命周期 = 用户"连接"到"断开"
- 功能：
  - 执行远程命令（挂载、卸载、首次 bootstrap）
  - 建立反向端口转发：`VPS:2222 → 127.0.0.1:<SFTP端口>`
  - 作为"启动 Claude"功能的间接通路（但新终端窗口会另起一个 ssh 进程，不复用此连接）

### 6. Claude 启动器
点击按钮后：
```
cmd.exe /c start "" powershell.exe -NoExit -Command "ssh -i <admin_key> -t root@vps 'cd ~/local-code && claude'"
```
- 新 PowerShell 窗口独立进程
- 可连续点开多个
- 不干扰主 exe 的隧道状态

## 运行时流程

### 首次运行 (Bootstrap)
1. 用户双击 exe → GUI 空白
2. 用户填入 VPS 地址、用户、端口、**密码**、共享目录
3. 点击"连接"
4. exe 检测 `bootstrapped == false`，进入 bootstrap：
   1. 生成 admin key pair，保存到 `./keys/admin_ed25519[.pub]`
   2. 用**密码**拨号 SSH 到 VPS
   3. 远程执行：
      ```bash
      mkdir -p ~/.ssh && chmod 700 ~/.ssh
      touch ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys
      grep -qxF '<admin pubkey>' ~/.ssh/authorized_keys || echo '<admin pubkey>' >> ~/.ssh/authorized_keys
      command -v sshfs >/dev/null || {
        apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y sshfs ||
        yum install -y fuse-sshfs
      }
      ```
   4. 标记 `bootstrapped = true`，保存 config，清空密码字段
5. 进入常规"连接"流程（见下）

### 每次"连接"
1. 用 admin key SSH 到 VPS（无密码），拿到一个 session
2. 生成临时 sftp access key pair
3. 启动本地 SFTP Server 于 `127.0.0.1:<随机端口>`，白名单 sftp pubkey
4. 在 admin SSH 通道上创建反向端口转发：`VPS:2222 → 127.0.0.1:<SFTP端口>`
5. 通过 admin SSH 通道执行：
   ```bash
   # 先写入 sftp 私钥
   umask 077
   cat > /tmp/.wasm-<rand> <<'KEOF'
   <sftp private key content>
   KEOF
   mkdir -p ~/local-code
   fusermount -u ~/local-code 2>/dev/null || true
   sshfs -p 2222 \
     -o IdentityFile=/tmp/.wasm-<rand> \
     -o StrictHostKeyChecking=no \
     -o UserKnownHostsFile=/dev/null \
     -o reconnect,ServerAliveInterval=15,ServerAliveCountMax=3 \
     -o cache=yes,compression=no \
     winshare@localhost:/ ~/local-code
   ```
   （`winshare` 是嵌入式 SFTP 服务的固定用户名，仅标识用，认证靠 key）
6. 等待返回码，成功 → GUI 状态变绿，启用"启动 Claude"按钮
7. 启动计时器，定期探测 admin SSH 是否活着（心跳）

### 点击"启动 Claude Code"
- spawn 新 PowerShell 窗口，执行：
  ```
  ssh -i <admin_key> -o StrictHostKeyChecking=no -t root@<vps> "cd ~/local-code && claude"
  ```
- 用户在新窗口里使用 claude
- 关闭窗口 ≠ 断开主连接

### 点击"断开"或关闭主窗口
1. Trap 关闭事件
2. 通过 admin SSH 执行：
   ```bash
   fusermount -u ~/local-code 2>/dev/null
   rm -f /tmp/.wasm-<rand>
   ```
3. 关闭反向转发
4. 停止本地 SFTP Server
5. 关闭 admin SSH 连接
6. 退出 exe

### 异常恢复
- 如果 admin SSH 心跳失败：GUI 显示"连接丢失"，尝试重建（最多 3 次），失败后置红灯、按钮禁用
- 如果 sshfs 挂载失败：日志显示 VPS 返回的 stderr，状态置红
- 如果 exe 被任务管理器强杀：VPS 上会残留 `/tmp/.wasm-<rand>`，下次连接时新的挂载步骤会用 `fusermount -u` 先清理旧挂载；临时私钥靠 tmpreaper/重启自然清理，也可以每次连接前扫一遍 `/tmp/.wasm-*` 清掉陈旧文件（>1 天）

## 安全模型

- **密码零存储**：首次 bootstrap 后立刻从内存和 GUI 字段清掉
- **admin key 静态存本地**：与 exe 同目录的 `./keys/` 下，依赖 Windows 文件系统权限保护（不加密）。**移动 exe 时同时移动 `keys/` 和 `config.json`**
- **sftp 私钥只存在于单次连接期间**：每次连接新生成、连接结束自动删除
- **SFTP 服务只监听 localhost**：外网绝对无法触达
- **反向隧道绑定 VPS localhost**：默认的 `GatewayPorts no` 保证只有 VPS 自己能访问 2222 端口

**已知弱点**：`./keys/admin_ed25519` 在 Windows 上是明文。搬迁 exe 时需要一并拷贝。可以后续版本考虑用 Windows DPAPI 封装。

## 构建 & 分发

### 开发环境
- 在 VPS (Linux) 上开发，使用 Go 交叉编译生成 Windows 可执行文件
- 依赖：
  - `fyne.io/fyne/v2`
  - `golang.org/x/crypto/ssh`
  - `github.com/pkg/sftp`

### 编译命令
```bash
cd /root/windows-auto-stimulator/app
GOOS=windows GOARCH=amd64 CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc \
  go build -ldflags "-s -w -H=windowsgui" -o wasm.exe .
```
- `CGO_ENABLED=1`：Fyne 依赖 GLFW (C 代码)，必须启用 CGO
- `x86_64-w64-mingw32-gcc`：需要在 VPS 上先装 `mingw-w64` 交叉编译器
- `-H=windowsgui`：运行时不弹控制台黑窗
- 产物约 20–30 MB

**备选**：若 VPS 上 mingw 装不了，可以用 `fyne-cross`（Docker 封装）代替手工 mingw。两种都只需在 VPS 跑。

### 分发
- 交付物：`wasm.exe` 单文件（**或 `wasm.exe` + `keys/` 目录**，首次运行后）
- 用户拷到任意目录运行；建议放在一个空文件夹里，exe 首次运行会在旁边生成 `keys/` 和 `config.json`

## 目录结构 (源码)

```
/root/windows-auto-stimulator/app/
├── go.mod
├── go.sum
├── main.go                 # 入口 + Fyne GUI 组装
├── config/
│   └── config.go           # config.json 读写
├── identity/
│   └── identity.go         # 密钥生成/加载 (避免与运行时 ./keys/ 目录重名)
├── sshclient/
│   ├── client.go           # admin SSH 客户端 + 端口转发
│   └── remote.go           # 远程命令封装 (bootstrap, mount, unmount)
├── sftpserver/
│   └── server.go           # 嵌入式 SFTP Server
├── claude/
│   └── launcher.go         # 启动 Claude 按钮的处理
└── ui/
    ├── main_window.go      # 主窗口布局
    ├── form.go             # 配置表单
    ├── status.go           # 状态面板
    └── log.go              # 日志显示
```

## 测试方案

### 手动测试矩阵
| 场景 | 预期 |
|---|---|
| 首次运行，密码正确 | bootstrap 成功，admin key 写入 VPS，sshfs 装好，挂载成功 |
| 首次运行，密码错误 | 红色错误提示，清空密码字段要求重输 |
| 二次运行，直接点连接 | 免密连接，秒级挂载成功 |
| 连接成功后点启动 Claude | 新 PowerShell 窗口开启，进入 VPS 的 `~/local-code`，claude 启动 |
| 连接成功后关闭 exe 主窗口 | VPS 挂载自动卸载，临时私钥删除 |
| 连接过程中 VPS 重启 | 心跳失败，GUI 提示连接丢失，手动重连恢复 |
| 强杀 exe 进程后再次启动 | 新连接前清理陈旧挂载，重新挂载成功 |
| 共享目录改动 | 保存后下次连接用新目录 (不支持热切换) |
| 在 Claude 里改 main.py | Windows 本地文件立即变化 |

### 测试环境
- 开发/联调：使用当前这台 VPS (172.236.229.37) 和一台 Windows 机器
- 不做单元测试（工具类项目，UI + 外部进程为主，ROI 低）
- 关键路径用 `go run` 手动跑，最后产物 exe 冒烟

## 风险与缓解

| 风险 | 可能性 | 影响 | 缓解 |
|---|---|---|---|
| Fyne 在老 Windows 7/8 上不兼容 | 低 | 无法运行 | 明确支持 Windows 10+ |
| mingw 交叉编译环境没准备好 | 中 | 构建失败 | 文档记录安装命令 |
| VPS 的 sshd 禁用了反向转发 (`AllowTcpForwarding no`) | 低 | 无法建隧道 | 明确报错、给出 sshd_config 修改建议 |
| 共享目录在 OneDrive/网盘同步目录 | 中 | 写入冲突、性能烂 | 文档提醒避开 |
| 大项目 sshfs 性能差 (grep/find) | 高 | 慢 | 文档建议 `.git`/`node_modules` 不要放挂载目录操作，或加 sshfs 缓存参数 |
| sshfs 断线后僵尸挂载 | 中 | VPS 进程卡住 | 连接前总是 `fusermount -u` 一次清理 |

## 未来可能的改进（本期不做）

- Windows DPAPI 加密保存 admin private key
- 多 VPS profile 切换
- 挂载点/共享目录历史记录
- 内嵌终端窗口（放弃外挂 PowerShell）
- 系统托盘图标最小化

---

## 待批准
本文档等待用户审阅。确认后进入 writing-plans 阶段生成实现计划。
