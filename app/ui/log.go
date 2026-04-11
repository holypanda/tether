package ui

import (
	"fmt"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

type LogPanel struct {
	mu        sync.Mutex
	entry     *widget.Entry
	container *fyne.Container
}

func NewLogPanel() *LogPanel {
	e := widget.NewMultiLineEntry()
	e.Wrapping = fyne.TextWrapWord
	e.Disable()
	scroll := container.NewVScroll(e)
	scroll.SetMinSize(fyne.NewSize(500, 150))
	return &LogPanel{
		entry:     e,
		container: container.NewStack(scroll),
	}
}

func (l *LogPanel) Container() *fyne.Container { return l.container }

func (l *LogPanel) Append(msg string) {
	ts := time.Now().Format("15:04:05")
	l.mu.Lock()
	cur := l.entry.Text
	if cur != "" {
		cur += "\n"
	}
	next := cur + fmt.Sprintf("[%s] %s", ts, msg)
	l.mu.Unlock()
	l.entry.SetText(next)
}

func (l *LogPanel) Error(format string, a ...any) {
	l.Append("[ERROR] " + fmt.Sprintf(format, a...))
}
