package ui

import (
	"fmt"
	"sync"

	"stim-link/config"
	"stim-link/identity"
	"stim-link/sftpserver"
	"stim-link/sshclient"
)

const configPath = "./stim-link.json"

type Connection struct {
	cfg         *config.Config
	admin       *identity.Identity
	sshClient   *sshclient.Client
	sftpSrv     *sftpserver.Server
	mountHandle *sshclient.MountHandle
	stopForward func()
	closeOnce   sync.Once
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
		RemoteMountPoint:  cfg.RemoteMountPoint,
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
		cfg:         cfg,
		admin:       admin,
		sshClient:   admClient,
		sftpSrv:     sftpSrv,
		mountHandle: handle,
		stopForward: stopForward,
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
