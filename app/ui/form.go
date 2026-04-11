package ui

import (
	"path/filepath"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"stim-link/config"
)

type ConfigForm struct {
	cfg *config.Config

	hostEntry  *widget.Entry
	userEntry  *widget.Entry
	portEntry  *widget.Entry
	passEntry  *widget.Entry
	shareEntry *widget.Entry
	mountEntry *widget.Entry

	container *fyne.Container
}

func NewConfigForm(cfg *config.Config, parent fyne.Window) *ConfigForm {
	f := &ConfigForm{cfg: cfg}

	f.hostEntry = widget.NewEntry()
	f.hostEntry.SetText(cfg.VpsHost)
	f.hostEntry.SetPlaceHolder("1.2.3.4")

	f.userEntry = widget.NewEntry()
	f.userEntry.SetText(cfg.VpsUser)

	f.portEntry = widget.NewEntry()
	f.portEntry.SetText(strconv.Itoa(cfg.VpsPort))

	f.passEntry = widget.NewPasswordEntry()
	f.passEntry.SetPlaceHolder("仅首次配置需要")

	f.shareEntry = widget.NewEntry()
	f.shareEntry.SetText(cfg.SharePath)
	f.shareEntry.SetPlaceHolder(`例: D:\my-project`)

	browseBtn := widget.NewButtonWithIcon("", theme.FolderOpenIcon(), func() {
		dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil || uri == nil {
				return
			}
			f.shareEntry.SetText(uriToFSPath(uri))
		}, parent)
	})
	shareRow := container.NewBorder(nil, nil, nil, browseBtn, f.shareEntry)

	f.mountEntry = widget.NewEntry()
	f.mountEntry.SetText(cfg.RemoteMountPoint)

	form := widget.NewForm(
		widget.NewFormItem("VPS 地址", f.hostEntry),
		widget.NewFormItem("VPS 用户", f.userEntry),
		widget.NewFormItem("VPS 端口", f.portEntry),
		widget.NewFormItem("VPS 密码", f.passEntry),
		widget.NewFormItem("共享目录", shareRow),
		widget.NewFormItem("远端挂载", f.mountEntry),
	)

	f.container = container.NewVBox(form)
	return f
}

// uriToFSPath converts a Fyne URI returned by the folder dialog into a native
// filesystem path. On Windows the URI looks like "file:///C:/foo/bar" and
// URI.Path() yields "/C:/foo/bar" — we strip the leading slash before the
// drive letter and normalise separators.
func uriToFSPath(uri fyne.URI) string {
	p := uri.Path()
	if len(p) >= 3 && p[0] == '/' && p[2] == ':' {
		p = p[1:]
	}
	return filepath.FromSlash(p)
}

func (f *ConfigForm) Container() *fyne.Container { return f.container }

// Snapshot writes the current UI values back into the Config struct and returns
// the password the user typed. The password is intentionally NOT stored in Config.
func (f *ConfigForm) Snapshot() (password string) {
	f.cfg.VpsHost = f.hostEntry.Text
	f.cfg.VpsUser = f.userEntry.Text
	if p, err := strconv.Atoi(f.portEntry.Text); err == nil {
		f.cfg.VpsPort = p
	}
	f.cfg.SharePath = f.shareEntry.Text
	f.cfg.RemoteMountPoint = f.mountEntry.Text
	return f.passEntry.Text
}

// ClearPassword wipes the password field after successful bootstrap so it is
// not retained in the UI after use.
func (f *ConfigForm) ClearPassword() {
	f.passEntry.SetText("")
}
