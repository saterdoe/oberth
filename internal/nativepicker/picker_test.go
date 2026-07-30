package nativepicker

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestPickDirectoryUsesPlatformNativePicker(t *testing.T) {
	tests := []struct {
		goos       string
		wantBinary string
		wantArg    string
	}{
		{goos: "windows", wantBinary: "powershell.exe", wantArg: "-STA"},
		{goos: "darwin", wantBinary: "osascript", wantArg: "choose folder"},
		{goos: "linux", wantBinary: "zenity", wantArg: "--directory"},
	}
	for _, test := range tests {
		t.Run(test.goos, func(t *testing.T) {
			var binary string
			var arguments []string
			selected, err := pickDirectory(
				context.Background(),
				test.goos,
				func(name string) (string, error) { return name, nil },
				func(_ context.Context, name string, args ...string) ([]byte, error) {
					binary, arguments = name, args
					return []byte("/workspace/repo\n"), nil
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			if selected != "/workspace/repo" {
				t.Fatalf("selected %q", selected)
			}
			if binary != test.wantBinary || !strings.Contains(strings.Join(arguments, " "), test.wantArg) {
				t.Fatalf("command %q %q", binary, arguments)
			}
		})
	}
}

func TestPickDirectoryFallsBackToKDialog(t *testing.T) {
	selected, err := pickDirectory(
		context.Background(),
		"linux",
		func(name string) (string, error) {
			if name == "zenity" {
				return "", errors.New("not found")
			}
			return name, nil
		},
		func(_ context.Context, name string, _ ...string) ([]byte, error) {
			if name != "kdialog" {
				t.Fatalf("used %q", name)
			}
			return []byte("/repo"), nil
		},
	)
	if err != nil || selected != "/repo" {
		t.Fatalf("selected %q, err %v", selected, err)
	}
}

func TestPickDirectoryReportsCancelAndUnsupportedPlatform(t *testing.T) {
	_, err := pickDirectory(
		context.Background(),
		"darwin",
		func(name string) (string, error) { return name, nil },
		func(context.Context, string, ...string) ([]byte, error) { return nil, errors.New("exit 1") },
	)
	if !errors.Is(err, ErrCanceled) {
		t.Fatalf("got %v", err)
	}

	_, err = pickDirectory(context.Background(), "plan9", nil, nil)
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("got %v", err)
	}
}
