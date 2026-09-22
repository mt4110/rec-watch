package launchagent

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveInstallOptionsRejectsGoRunBinary(t *testing.T) {
	_, err := ResolveInstallOptions(InstallOptions{HomeDir: "/Users/me"}, filepath.Join(os.TempDir(), "go-build123", "b001", "exe", "rec-watch"))
	if err == nil {
		t.Fatal("expected temporary go run binary to be rejected")
	}
}

func TestRenderPlistIncludesStableLaunchdContract(t *testing.T) {
	data := PlistData{
		Label:      Label,
		BinaryPath: "/usr/local/bin/rec-watch",
		WatchDir:   "/Users/me/Desktop/ScreenRecordings",
		DestDir:    "/Users/me/Desktop/ScreenRecordings-out",
		WorkDir:    "/Users/me/Desktop/ScreenRecordings-out",
		LogPath:    "/Users/me/Library/Logs/rec-watch.log",
	}

	got, err := RenderPlist(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := xml.Unmarshal([]byte(got), new(any)); err != nil {
		t.Fatalf("plist is not well-formed XML: %v", err)
	}
	for _, want := range []string{
		"<string>/usr/local/bin/rec-watch</string>",
		"<string>--watch</string>",
		"<string>/Users/me/Desktop/ScreenRecordings</string>",
		"<string>--dest</string>",
		"<string>/Users/me/Desktop/ScreenRecordings-out</string>",
		"<string>--source-policy</string>",
		"<string>keep</string>",
		"<key>WorkingDirectory</key>",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("plist does not contain %q\n%s", want, got)
		}
	}
}

func TestLaunchctlArgsUseModernGuiDomain(t *testing.T) {
	uid := 501
	plistPath := "/Users/me/Library/LaunchAgents/com.user.recwatch.plist"

	assertArgs(t, BootstrapArgs(uid, plistPath), []string{"bootstrap", "gui/501", plistPath})
	assertArgs(t, BootoutArgs(uid), []string{"bootout", "gui/501/com.user.recwatch"})
	assertArgs(t, KickstartArgs(uid), []string{"kickstart", "-k", "gui/501/com.user.recwatch"})
	assertArgs(t, PrintArgs(uid), []string{"print", "gui/501/com.user.recwatch"})
}

func assertArgs(t *testing.T, got, want []string) {
	t.Helper()
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("args = %q, want %q", got, want)
	}
}
