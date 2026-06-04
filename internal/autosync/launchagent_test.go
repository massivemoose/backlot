package autosync

import (
	"encoding/xml"
	"strings"
	"testing"
)

func TestRenderLaunchAgentIncludesManagedRunnerAndSchedule(t *testing.T) {
	paths := Paths{
		Label:     "com.massivemoose.backlot.autosync.test",
		Root:      `/Users/me/Backlot & Notes`,
		LogPath:   `/Users/me/Library/Logs/Backlot/error.log`,
		PlistPath: `/Users/me/Library/LaunchAgents/test.plist`,
	}
	config := Config{
		Binary:          `/Applications/Backlot & Tools/backlot`,
		IntervalSeconds: 900,
	}
	data, err := RenderLaunchAgent(paths, config)
	if err != nil {
		t.Fatalf("RenderLaunchAgent returned error: %v", err)
	}
	var parsed any
	if err := xml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("RenderLaunchAgent produced invalid XML: %v\n%s", err, data)
	}
	text := string(data)
	for _, want := range []string{
		"<key>Label</key>",
		"<string>com.massivemoose.backlot.autosync.test</string>",
		"<string>/Applications/Backlot &amp; Tools/backlot</string>",
		"<string>autosync</string>",
		"<string>run</string>",
		"<string>--root</string>",
		"<string>/Users/me/Backlot &amp; Notes</string>",
		"<key>RunAtLoad</key>",
		"<true/>",
		"<key>StartInterval</key>",
		"<integer>900</integer>",
		"<key>ProcessType</key>",
		"<string>Background</string>",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("LaunchAgent missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "<true></true>") {
		t.Fatalf("LaunchAgent uses expanded plist boolean rejected by launchd:\n%s", text)
	}
}
