#Requires -Version 5.0
<#
    一键脚本: 反向 SSH 隧道 (Windows 端)
    - 首次运行: 自动安装 OpenSSH Server, 生成密钥, 交换公钥, 写配置, 启动隧道
    - 再次运行: 直接启动隧道, 断线重连
    - 只在首次配置时问一次 VPS 密码
#>

param(
    [string]$VpsHost,
    [string]$VpsUser = "root",
    [int]$VpsPort = 22,
    [int]$RemotePort = 2222,
    [string]$SharePath,
    [switch]$Reconfigure
)

$ErrorActionPreference = "Stop"
$scriptDir  = Split-Path -Parent $MyInvocation.MyCommand.Path
$configPath = Join-Path $scriptDir "config.json"
$logPath    = Join-Path $scriptDir "tunnel.log"

# 所有输出同时写日志, 脚本再崩也能回看
Start-Transcript -Path $logPath -Append -Force | Out-Null

# 顶层错误捕获: 出错时打印详情并保持窗口打开
trap {
    Write-Host ""
    Write-Host "====================================" -ForegroundColor Red
    Write-Host "[FATAL] 脚本异常退出" -ForegroundColor Red
    Write-Host "错误信息: $($_.Exception.Message)" -ForegroundColor Red
    Write-Host "位置: $($_.InvocationInfo.PositionMessage)" -ForegroundColor DarkRed
    Write-Host "调用栈:" -ForegroundColor DarkRed
    Write-Host $_.ScriptStackTrace -ForegroundColor DarkGray
    Write-Host "====================================" -ForegroundColor Red
    Write-Host "完整日志: $logPath" -ForegroundColor Yellow
    Stop-Transcript | Out-Null
    Read-Host "按回车键退出"
    exit 1
}

# 共享目录默认 = 脚本所在目录 (把脚本放进项目, 直接共享整个项目)
if (-not $SharePath) { $SharePath = $scriptDir }

function Write-Step($msg) { Write-Host "[*] $msg" -ForegroundColor Cyan }
function Write-OK($msg)   { Write-Host "[OK] $msg" -ForegroundColor Green }
function Write-Err($msg)  { Write-Host "[ERROR] $msg" -ForegroundColor Red }
function Test-Admin {
    $id = [Security.Principal.WindowsIdentity]::GetCurrent()
    ([Security.Principal.WindowsPrincipal]$id).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

# 自动提权: 首次配置需要管理员权限, 缺则自动以管理员身份重启 (新窗口 -NoExit 保活)
$needBootstrap = (-not (Test-Path $configPath)) -or $Reconfigure
if ($needBootstrap -and -not (Test-Admin)) {
    Write-Host "[*] 首次配置需要管理员权限, 正在以管理员身份重启..." -ForegroundColor Yellow
    Stop-Transcript | Out-Null
    $psExe = (Get-Process -Id $PID).Path
    $argList = @('-NoProfile','-ExecutionPolicy','Bypass','-NoExit','-File',"`"$PSCommandPath`"")
    try {
        Start-Process -FilePath $psExe -ArgumentList $argList -Verb RunAs
    } catch {
        Write-Host "[ERROR] 提权失败: $($_.Exception.Message)" -ForegroundColor Red
        Read-Host "按回车退出"
    }
    exit
}

# ----------------- 加载/初始化配置 -----------------
if ((Test-Path $configPath) -and -not $Reconfigure) {
    $cfg = Get-Content $configPath -Raw | ConvertFrom-Json
} else {
    if (-not $VpsHost) { $VpsHost = Read-Host "VPS 地址 (IP 或域名)" }
    Write-Host "共享目录: $SharePath" -ForegroundColor Green
    $cfg = [PSCustomObject]@{
        VpsHost      = $VpsHost
        VpsUser      = $VpsUser
        VpsPort      = $VpsPort
        RemotePort   = $RemotePort
        SharePath    = $SharePath
        WinUser      = $env:USERNAME
        Bootstrapped = $false
    }
}

# ----------------- 首次配置 -----------------
if (-not $cfg.Bootstrapped) {
    if (-not (Test-Admin)) {
        throw "首次配置需要管理员权限 (理论上应已自动提权, 如看到此错误请手动右键 tunnel.bat -> 以管理员身份运行)"
    }

    Write-Host ""
    Write-Host "========== 首次配置 ==========" -ForegroundColor Magenta

    # 1. 安装并启动 OpenSSH Server
    Write-Step "检查 OpenSSH Server"
    $feat = Get-WindowsCapability -Online | Where-Object { $_.Name -like "OpenSSH.Server*" } | Select-Object -First 1
    if ($feat -and $feat.State -ne "Installed") {
        Write-Step "安装 OpenSSH Server (可能需要几分钟)"
        Add-WindowsCapability -Online -Name $feat.Name | Out-Null
    }
    Set-Service -Name sshd -StartupType Automatic
    Start-Service sshd -ErrorAction SilentlyContinue
    if (-not (Get-NetFirewallRule -Name sshd -ErrorAction SilentlyContinue)) {
        New-NetFirewallRule -Name sshd -DisplayName 'OpenSSH Server' -Enabled True `
            -Direction Inbound -Protocol TCP -Action Allow -LocalPort 22 | Out-Null
    }
    Write-OK "sshd 已运行"

    # 2. 生成 Windows SSH 密钥
    Write-Step "检查本机 SSH 密钥"
    $sshDir    = Join-Path $env:USERPROFILE ".ssh"
    $winKey    = Join-Path $sshDir "id_ed25519"
    $winPubKey = "$winKey.pub"
    if (-not (Test-Path $sshDir)) { New-Item -ItemType Directory -Path $sshDir | Out-Null }
    if (-not (Test-Path $winKey)) {
        & ssh-keygen -t ed25519 -f $winKey -N '""' -q | Out-Null
    }
    $winPub = (Get-Content $winPubKey -Raw).Trim()
    Write-OK "本机公钥就绪"

    # 3. 构造远端初始化脚本 (base64 传输避免转义问题)
    $winPath = $cfg.SharePath
    # D:\foo\bar  ->  /D:/foo/bar   (OpenSSH on Windows 兼容格式)
    $winPathUnix = ($winPath -replace '\\', '/')
    if ($winPathUnix -match '^([A-Za-z]):') { $winPathUnix = "/$($winPathUnix)" }

    $remoteScript = @"
set -e
mkdir -p ~/.ssh && chmod 700 ~/.ssh
touch ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys
grep -qxF '$winPub' ~/.ssh/authorized_keys || echo '$winPub' >> ~/.ssh/authorized_keys
[ -f ~/.ssh/id_ed25519 ] || ssh-keygen -t ed25519 -f ~/.ssh/id_ed25519 -N '' -q
if ! command -v sshfs >/dev/null 2>&1; then
  apt-get update -qq >/dev/null 2>&1 || true
  DEBIAN_FRONTEND=noninteractive apt-get install -y sshfs >/dev/null 2>&1 || \
  yum install -y fuse-sshfs >/dev/null 2>&1 || true
fi
cat > ~/.windows-auto-stim.env <<'CFGEOF'
WIN_USER='$($cfg.WinUser)'
WIN_PATH='$winPathUnix'
MOUNT_POINT="`$HOME/local-code"
TUNNEL_PORT=$($cfg.RemotePort)
CFGEOF
echo '---VPS_PUBKEY_BEGIN---'
cat ~/.ssh/id_ed25519.pub
echo '---VPS_PUBKEY_END---'
"@

    $bytes = [Text.Encoding]::UTF8.GetBytes($remoteScript)
    $b64   = [Convert]::ToBase64String($bytes)
    $sshCmd = "echo $b64 | base64 -d | bash"

    Write-Host ""
    Write-Host "即将连接 VPS 执行初始化 (会提示输入一次 VPS 密码):" -ForegroundColor Yellow
    Write-Host ""

    $output = & ssh -o StrictHostKeyChecking=accept-new `
                    -o PreferredAuthentications=password,keyboard-interactive,publickey `
                    -p $cfg.VpsPort `
                    "$($cfg.VpsUser)@$($cfg.VpsHost)" `
                    $sshCmd 2>&1
    if ($LASTEXITCODE -ne 0) {
        Write-Err "VPS 初始化失败, 退出码 $LASTEXITCODE"
        $output | ForEach-Object { Write-Host $_ }
        exit 1
    }

    # 提取 VPS 公钥
    $vpsPub = $null
    $capture = $false
    foreach ($line in $output) {
        if ($line -match '---VPS_PUBKEY_BEGIN---') { $capture = $true; continue }
        if ($line -match '---VPS_PUBKEY_END---')   { break }
        if ($capture -and $line -match '^ssh-') { $vpsPub = $line.Trim(); break }
    }
    if (-not $vpsPub) {
        Write-Err "未能获取 VPS 公钥"
        exit 1
    }
    Write-OK "已获取 VPS 公钥"

    # 4. 把 VPS 公钥加入 Windows authorized_keys (管理员走 administrators_authorized_keys)
    Write-Step "写入 VPS 公钥到 Windows"
    $userGroups = (whoami /groups) 2>$null
    $isAdminUser = $userGroups -match 'S-1-5-32-544'

    if ($isAdminUser) {
        $authPath = "$env:ProgramData\ssh\administrators_authorized_keys"
    } else {
        $authPath = Join-Path $sshDir "authorized_keys"
    }
    $authDir = Split-Path $authPath -Parent
    if (-not (Test-Path $authDir)) { New-Item -ItemType Directory -Path $authDir -Force | Out-Null }

    $existing = if (Test-Path $authPath) { (Get-Content $authPath -Raw) } else { "" }
    if ($existing -notlike "*$vpsPub*") {
        Add-Content -Path $authPath -Value $vpsPub
    }

    # 修复 ACL (sshd 对权限很挑)
    if ($isAdminUser) {
        & icacls.exe $authPath /inheritance:r /grant "Administrators:F" /grant "SYSTEM:F" | Out-Null
    } else {
        & icacls.exe $authPath /inheritance:r /grant "$($env:USERNAME):F" /grant "SYSTEM:F" | Out-Null
    }
    Write-OK "已写入 $authPath"

    # 5. 保存本地配置
    $cfg.Bootstrapped = $true
    $cfg | ConvertTo-Json | Set-Content -Path $configPath -Encoding UTF8
    Write-OK "配置已保存: $configPath"
    Write-Host ""
    Write-Host "========== 首次配置完成 ==========" -ForegroundColor Magenta
    Write-Host ""
}

# ----------------- 启动隧道循环 -----------------
Write-Host "====================================" -ForegroundColor Cyan
Write-Host " 反向隧道: localhost:22 -> $($cfg.VpsHost):$($cfg.RemotePort)" -ForegroundColor Cyan
Write-Host " 在 VPS 上运行: ~/windows-auto-stimulator/vps/mount.sh" -ForegroundColor Cyan
Write-Host " 按 Ctrl+C 退出" -ForegroundColor Cyan
Write-Host "====================================" -ForegroundColor Cyan

$winKey = Join-Path $env:USERPROFILE ".ssh\id_ed25519"

while ($true) {
    $ts = Get-Date -Format "HH:mm:ss"
    Write-Host "[$ts] 连接中..." -ForegroundColor Yellow

    & ssh -N `
        -i $winKey `
        -o ExitOnForwardFailure=yes `
        -o ServerAliveInterval=30 `
        -o ServerAliveCountMax=3 `
        -o StrictHostKeyChecking=accept-new `
        -o BatchMode=yes `
        -R "$($cfg.RemotePort):localhost:22" `
        -p $cfg.VpsPort `
        "$($cfg.VpsUser)@$($cfg.VpsHost)"

    $ts = Get-Date -Format "HH:mm:ss"
    Write-Host "[$ts] 断开, 5s 后重连..." -ForegroundColor Red
    Start-Sleep -Seconds 5
}
