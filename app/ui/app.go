package ui

import (
	"fyne.io/fyne/v2"
	fyneapp "fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"stim-link/config"
)

type UI struct {
	app          fyne.App
	window       fyne.Window
	cfg          *config.Config
	form         *ConfigForm
	status       *StatusPanel
	logPanel     *LogPanel
	connectBtn   *widget.Button
	disconnectBtn *widget.Button
	conn         *Connection
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
	cfg, _ := config.Load(configPath)
	u.cfg = cfg
	u.form = NewConfigForm(cfg)
	u.status = NewStatusPanel()
	u.logPanel = NewLogPanel()

	u.connectBtn = widget.NewButton("🔌 连接", u.onConnect)
	u.disconnectBtn = widget.NewButton("⏹ 断开", u.onDisconnect)
	u.disconnectBtn.Disable()

	buttons := container.NewHBox(u.connectBtn, u.disconnectBtn)

	content := container.NewVBox(
		widget.NewLabelWithStyle("连接配置", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		u.form.Container(),
		buttons,
		widget.NewSeparator(),
		widget.NewLabelWithStyle("状态", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		u.status.Container(),
		widget.NewSeparator(),
		widget.NewLabelWithStyle("日志", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		u.logPanel.Container(),
	)
	u.window.SetContent(content)

	u.window.SetCloseIntercept(func() {
		if u.conn != nil {
			u.logPanel.Append("窗口关闭, 清理中...")
			u.conn.Disconnect(u.logPanel.Append)
			u.conn = nil
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
		u.conn = conn
		u.form.ClearPassword()
		_ = config.Save(configPath, u.cfg)
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

func (u *UI) Run() {
	u.window.ShowAndRun()
}
