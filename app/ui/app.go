package ui

import (
	"fyne.io/fyne/v2"
	fyneapp "fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"stim-link/config"
)

type UI struct {
	app    fyne.App
	window fyne.Window
	cfg    *config.Config
	form   *ConfigForm
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
	cfg, _ := config.Load("./config.json")
	u.cfg = cfg
	u.form = NewConfigForm(cfg)

	content := container.NewVBox(
		widget.NewLabelWithStyle("连接配置", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		u.form.Container(),
	)
	u.window.SetContent(content)
}

func (u *UI) Run() {
	u.window.ShowAndRun()
}
