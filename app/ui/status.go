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
