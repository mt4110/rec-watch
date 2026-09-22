package launchagent

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
)

const Label = "com.user.recwatch"

type Paths struct {
	HomeDir   string
	PlistPath string
	LogPath   string
}

type InstallOptions struct {
	HomeDir    string
	BinaryPath string
	WatchDir   string
	DestDir    string
}

type PlistData struct {
	Label      string
	BinaryPath string
	WatchDir   string
	DestDir    string
	WorkDir    string
	LogPath    string
}

func DefaultPaths(home string) Paths {
	return Paths{
		HomeDir:   home,
		PlistPath: filepath.Join(home, "Library", "LaunchAgents", Label+".plist"),
		LogPath:   filepath.Join(home, "Library", "Logs", "rec-watch.log"),
	}
}

func DefaultWatchDir(home string) string {
	return filepath.Join(home, "Desktop", "ScreenRecordings")
}

func DefaultDestDir(home string) string {
	return filepath.Join(home, "Desktop", "ScreenRecordings-out")
}

func ResolveInstallOptions(opt InstallOptions, currentExecutable string) (PlistData, error) {
	home := opt.HomeDir
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return PlistData{}, fmt.Errorf("home directory: %w", err)
		}
	}

	binaryPath := opt.BinaryPath
	if binaryPath == "" {
		binaryPath = currentExecutable
	}
	if binaryPath == "" {
		return PlistData{}, errors.New("binary path is required")
	}
	if !filepath.IsAbs(binaryPath) {
		resolved, err := exec.LookPath(binaryPath)
		if err != nil {
			return PlistData{}, fmt.Errorf("resolve binary path %q: %w", binaryPath, err)
		}
		binaryPath = resolved
	}
	binaryPath = filepath.Clean(binaryPath)
	if isGoRunBinary(binaryPath) {
		return PlistData{}, fmt.Errorf("refusing to install temporary go run binary %q; install rec-watch first or pass --bin /absolute/path/to/rec-watch", binaryPath)
	}

	watchDir := opt.WatchDir
	if watchDir == "" {
		watchDir = DefaultWatchDir(home)
	}
	destDir := opt.DestDir
	if destDir == "" {
		destDir = DefaultDestDir(home)
	}

	paths := DefaultPaths(home)
	return PlistData{
		Label:      Label,
		BinaryPath: binaryPath,
		WatchDir:   filepath.Clean(expandHome(watchDir, home)),
		DestDir:    filepath.Clean(expandHome(destDir, home)),
		WorkDir:    filepath.Clean(expandHome(destDir, home)),
		LogPath:    paths.LogPath,
	}, nil
}

func EnsureInstallDirs(data PlistData, paths Paths) error {
	for _, dir := range []string{
		filepath.Dir(paths.PlistPath),
		filepath.Dir(data.LogPath),
		data.WatchDir,
		data.DestDir,
		data.WorkDir,
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create directory %s: %w", dir, err)
		}
	}
	return nil
}

func WritePlist(path string, data PlistData) error {
	var buf bytes.Buffer
	if err := plistTemplate.Execute(&buf, data); err != nil {
		return err
	}
	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("write plist: %w", err)
	}
	return nil
}

func RenderPlist(data PlistData) (string, error) {
	var buf bytes.Buffer
	if err := plistTemplate.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func BootstrapArgs(uid int, plistPath string) []string {
	return []string{"bootstrap", guiDomain(uid), plistPath}
}

func BootoutArgs(uid int) []string {
	return []string{"bootout", guiDomain(uid) + "/" + Label}
}

func KickstartArgs(uid int) []string {
	return []string{"kickstart", "-k", guiDomain(uid) + "/" + Label}
}

func PrintArgs(uid int) []string {
	return []string{"print", guiDomain(uid) + "/" + Label}
}

func guiDomain(uid int) string {
	return "gui/" + strconv.Itoa(uid)
}

func RunLaunchctl(args []string) ([]byte, error) {
	return exec.Command("launchctl", args...).CombinedOutput()
}

func expandHome(path, home string) string {
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path
}

func isGoRunBinary(path string) bool {
	base := filepath.Base(path)
	if strings.HasPrefix(base, "___go_build") {
		return true
	}
	tmp := filepath.Clean(os.TempDir()) + string(os.PathSeparator)
	return strings.HasPrefix(filepath.Clean(path), tmp)
}

func xmlEscape(s string) string {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(s))
	return buf.String()
}

var plistTemplate = template.Must(template.New("plist").Funcs(template.FuncMap{
	"xml": xmlEscape,
}).Parse(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>{{xml .Label}}</string>
    <key>ProgramArguments</key>
    <array>
        <string>{{xml .BinaryPath}}</string>
        <string>--watch</string>
        <string>{{xml .WatchDir}}</string>
        <string>--dest</string>
        <string>{{xml .DestDir}}</string>
        <string>--source-policy</string>
        <string>keep</string>
    </array>
    <key>WorkingDirectory</key>
    <string>{{xml .WorkDir}}</string>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>{{xml .LogPath}}</string>
    <key>StandardErrorPath</key>
    <string>{{xml .LogPath}}</string>
</dict>
</plist>
`))
