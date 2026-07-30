package nativepicker

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

var (
	ErrCanceled    = errors.New("directory selection canceled")
	ErrUnavailable = errors.New("native directory picker unavailable")
)

type commandRunner func(context.Context, string, ...string) ([]byte, error)

func PickDirectory(ctx context.Context) (string, error) {
	return pickDirectory(ctx, runtime.GOOS, exec.LookPath, runCommand)
}

func pickDirectory(
	ctx context.Context,
	goos string,
	lookPath func(string) (string, error),
	run commandRunner,
) (string, error) {
	var name string
	var args []string

	switch goos {
	case "windows":
		path, err := lookPath("powershell.exe")
		if err != nil {
			return "", ErrUnavailable
		}
		name = path
		args = []string{"-NoLogo", "-NoProfile", "-STA", "-Command", windowsPickerScript}
	case "darwin":
		path, err := lookPath("osascript")
		if err != nil {
			return "", ErrUnavailable
		}
		name = path
		args = []string{"-e", `POSIX path of (choose folder with prompt "Selecciona un repositorio Git")`}
	case "linux":
		if path, err := lookPath("zenity"); err == nil {
			name = path
			args = []string{"--file-selection", "--directory", "--title=Selecciona un repositorio Git"}
		} else if path, err := lookPath("kdialog"); err == nil {
			name = path
			args = []string{"--getexistingdirectory", ".", "--title", "Selecciona un repositorio Git"}
		} else {
			return "", ErrUnavailable
		}
	default:
		return "", ErrUnavailable
	}

	output, err := run(ctx, name, args...)
	selected := strings.TrimSpace(string(output))
	if err != nil {
		if selected == "" {
			return "", ErrCanceled
		}
		return "", fmt.Errorf("open native directory picker: %w", err)
	}
	if selected == "" {
		return "", ErrCanceled
	}
	return selected, nil
}

func runCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

const windowsPickerScript = `
Add-Type -AssemblyName System.Windows.Forms
$owner = New-Object System.Windows.Forms.Form
$owner.Text = 'oberth'
$owner.ShowInTaskbar = $false
$owner.TopMost = $true
$owner.StartPosition = [System.Windows.Forms.FormStartPosition]::CenterScreen
$owner.Size = New-Object System.Drawing.Size(1, 1)
$owner.Opacity = 0
$dialog = New-Object System.Windows.Forms.FolderBrowserDialog
$dialog.Description = 'Selecciona un repositorio Git'
$dialog.ShowNewFolderButton = $false
try {
  $owner.Show()
  $owner.Activate()
  $owner.BringToFront()
  if ($dialog.ShowDialog($owner) -eq [System.Windows.Forms.DialogResult]::OK) {
    [Console]::OutputEncoding = [System.Text.Encoding]::UTF8
    Write-Output $dialog.SelectedPath
  } else {
    exit 2
  }
} finally {
  $dialog.Dispose()
  $owner.Close()
  $owner.Dispose()
}`
