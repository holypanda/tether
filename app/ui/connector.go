package ui

import (
	"fmt"
	"strings"
	"sync"

	"tether/config"
	"tether/identity"
	"tether/sftpserver"
	"tether/sshclient"
)

const configPath = "./tether.json"

type Connection struct {
	cfg              *config.Config
	admin            *identity.Identity
	sshClient        *sshclient.Client
	sftpSrv          *sftpserver.Server
	mountHandle      *sshclient.MountHandle
	stopForward      func()
	closeOnce        sync.Once
	ResolvedMountDir string // absolute VPS path, with ~ expanded
}

// resolveRemoteMountPoint expands a leading `~` in p by asking the remote shell
// for $HOME. If p does not start with `~`, it is returned unchanged.
// Bash's ~ expansion only works when the path is NOT inside single quotes, and
// our Mount script bash-quotes the path for injection safety, so we have to
// resolve it client-side before quoting.
func resolveRemoteMountPoint(c *sshclient.Client, p string) (string, error) {
	if p == "" {
		return "", fmt.Errorf("remote mount point is empty")
	}
	if !strings.HasPrefix(p, "~") {
		return p, nil
	}
	out, err := c.Run(`printf '%s' "$HOME"`)
	if err != nil {
		return "", fmt.Errorf("probe remote HOME: %w", err)
	}
	home := strings.TrimSpace(string(out))
	if home == "" {
		return "", fmt.Errorf("remote HOME is empty")
	}
	if p == "~" {
		return home, nil
	}
	if strings.HasPrefix(p, "~/") {
		return home + "/" + strings.TrimPrefix(p, "~/"), nil
	}
	// ~user form not supported; caller should pass an absolute path instead.
	return "", fmt.Errorf("unsupported path form %q (only ~ and ~/... are expanded)", p)
}

// Connect runs the full bring-up sequence. On first run (cfg.Bootstrapped == false)
// it requires password to add adminKey and install sshfs, then proceeds. On later
// runs password can be empty and key auth is used directly.
// The `log` callback is called with progress messages so the UI can show them.
func Connect(cfg *config.Config, password string, log func(string)) (*Connection, error) {
	log("加载 admin 密钥...")
	admin, err := identity.GenerateOrLoad(cfg.AdminKeyPath)
	if err != nil {
		return nil, fmt.Errorf("load admin key: %w", err)
	}

	// First-time bootstrap via password
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
		if err := config.Save(configPath, cfg); err != nil {
			return nil, fmt.Errorf("save config: %w", err)
		}
		log("首次配置完成")
	}

	// Key-based login for the live session
	log("用密钥登录 VPS...")
	admClient, err := sshclient.DialKey(cfg.VpsHost, cfg.VpsPort, cfg.VpsUser, admin)
	if err != nil {
		return nil, fmt.Errorf("密钥登录失败: %w", err)
	}

	// Resolve ~ in the configured mount point before handing it to Mount,
	// because Mount single-quotes the path (injection-proof) and bash does
	// not expand ~ inside single quotes.
	resolvedMount, err := resolveRemoteMountPoint(admClient, cfg.RemoteMountPoint)
	if err != nil {
		admClient.Close()
		return nil, fmt.Errorf("解析远端挂载路径失败: %w", err)
	}
	log("远端挂载路径: " + resolvedMount)

	// Local embedded SFTP server
	log("启动本地 SFTP 服务...")
	sftpHost, err := identity.Ephemeral()
	if err != nil {
		admClient.Close()
		return nil, fmt.Errorf("gen sftp host key: %w", err)
	}
	sftpClient, err := identity.Ephemeral()
	if err != nil {
		admClient.Close()
		return nil, fmt.Errorf("gen sftp client key: %w", err)
	}
	sftpSrv, err := sftpserver.Start(sftpserver.Config{
		RootDir:          cfg.SharePath,
		HostIdentity:     sftpHost,
		AllowedClientKey: sftpClient.PublicKey(),
	})
	if err != nil {
		admClient.Close()
		return nil, fmt.Errorf("SFTP 启动失败: %w", err)
	}

	// Reverse tunnel: VPS:RemoteTunnelPort -> local SFTP port
	log(fmt.Sprintf("建立反向隧道 VPS:%d -> 本地:%d", cfg.RemoteTunnelPort, sftpSrv.Port()))
	remoteAddr := fmt.Sprintf("localhost:%d", cfg.RemoteTunnelPort)
	localAddr := fmt.Sprintf("127.0.0.1:%d", sftpSrv.Port())
	stopForward, err := admClient.ReverseForward(remoteAddr, localAddr)
	if err != nil {
		_ = sftpSrv.Close()
		admClient.Close()
		return nil, fmt.Errorf("反向转发失败: %w", err)
	}

	// Remote mount
	log("在 VPS 上执行 sshfs 挂载...")
	handle, err := admClient.Mount(sshclient.MountParams{
		WinShareUser:      "winshare",
		RemoteTunnelPort:  cfg.RemoteTunnelPort,
		RemoteMountPoint:  resolvedMount,
		SFTPPrivateKeyPEM: sftpClient.PrivatePEM(),
	})
	if err != nil {
		stopForward()
		_ = sftpSrv.Close()
		admClient.Close()
		return nil, err
	}
	log("挂载成功")
	return &Connection{
		cfg:              cfg,
		admin:            admin,
		sshClient:        admClient,
		sftpSrv:          sftpSrv,
		mountHandle:      handle,
		stopForward:      stopForward,
		ResolvedMountDir: resolvedMount,
	}, nil
}

// Disconnect tears down all resources in reverse order.
// Safe to call on a nil Connection; idempotent via sync.Once.
func (c *Connection) Disconnect(log func(string)) {
	if c == nil {
		return
	}
	c.closeOnce.Do(func() {
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
	})
}

// AdminKeyPath exposes the admin key path for the Claude launcher.
func (c *Connection) AdminKeyPath() string { return c.cfg.AdminKeyPath }
