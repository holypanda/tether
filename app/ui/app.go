package ui

import (
	"sync"

	"fyne.io/fyne/v2"
	fyneapp "fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"stim-link/claude"
	"stim-link/config"
)

type UI struct {
	app           fyne.App
	window        fyne.Window
	cfg           *config.Config
	form          *ConfigForm
	status        *StatusPanel
	logPanel      *LogPanel
	connectBtn    *widget.Button
	disconnectBtn *widget.Button
	claudeBtn     *widget.Button
	connMu        sync.Mutex
	conn          *Connection
}

func (u *UI) setConn(c *Connection) {
	u.connMu.Lock()
	u.conn = c
	u.connMu.Unlock()
}

func (u *UI) takeConn() *Connection {
	u.connMu.Lock()
	c := u.conn
	u.conn = nil
	u.connMu.Unlock()
	return c
}

func New() *UI {
	a := fyneapp.New()
	w := a.NewWindow("stim-link")
	w.Resize(fyne.NewSize(560, 560))
	u := &UI{app: a, window: w}
	u.build()
	return u
}

func (u *UI) build() {
	cfg, loadErr := config.Load(configPath)
	u.cfg = cfg
	u.form = NewConfigForm(cfg, u.window)
	u.status = NewStatusPanel()
	u.logPanel = NewLogPanel()
	if loadErr != nil {
		u.logPanel.Error("加载 %s 失败, 已使用默认值: %v", configPath, loadErr)
	}

	u.connectBtn = widget.NewButton("🔌 连接", u.onConnect)
	u.disconnectBtn = widget.NewButton("⏹ 断开", u.onDisconnect)
	u.disconnectBtn.Disable()

	buttons := container.NewHBox(u.connectBtn, u.disconnectBtn)

	u.claudeBtn = widget.NewButton("🚀 启动 Claude Code", u.onLaunchClaude)
	u.claudeBtn.Disable()

	content := container.NewVBox(
		widget.NewLabelWithStyle("连接配置", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		u.form.Container(),
		buttons,
		widget.NewSeparator(),
		widget.NewLabelWithStyle("状态", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		u.status.Container(),
		u.claudeBtn,
		widget.NewSeparator(),
		widget.NewLabelWithStyle("日志", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		u.logPanel.Container(),
	)
	u.window.SetContent(content)

	u.window.SetCloseIntercept(func() {
		if c := u.takeConn(); c != nil {
			u.logPanel.Append("窗口关闭, 清理中...")
			c.Disconnect(u.logPanel.Append)
		}
		u.window.Close()
	})
}

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
		u.setConn(conn)
		u.form.ClearPassword()
		if err := config.Save(configPath, u.cfg); err != nil {
			u.logPanel.Error("保存配置失败: %v", err)
		}
		u.status.Set(StateConnected)
		u.disconnectBtn.Enable()
		u.claudeBtn.Enable()
	}()
}

func (u *UI) onDisconnect() {
	u.disconnectBtn.Disable()
	go func() {
		c := u.takeConn()
		if c == nil {
			return
		}
		c.Disconnect(u.logPanel.Append)
		u.status.Set(StateDisconnected)
		u.connectBtn.Enable()
		u.claudeBtn.Disable()
	}()
}

func (u *UI) onLaunchClaude() {
	u.connMu.Lock()
	conn := u.conn
	u.connMu.Unlock()
	if conn == nil {
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

func (u *UI) Run() {
	u.window.ShowAndRun()
}
