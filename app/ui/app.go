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
	w := a.NewWindow("stim-link")
	w.Resize(fyne.NewSize(560, 560))
	u := &UI{app: a, window: w}
	u.build()
	return u
}

func (u *UI) build() {
	placeholder := widget.NewLabel("stim-link starting...")
	u.window.SetContent(container.NewCenter(placeholder))
}

func (u *UI) Run() {
	u.window.ShowAndRun()
}
