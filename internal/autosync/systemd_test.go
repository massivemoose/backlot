package autosync

import (
	"strings"
	"testing"
)

func TestRenderSystemdUserUnitsIncludeManagedRunnerAndSchedule(t *testing.T) {
	paths := Paths{
		Label:       "com.massivemoose.backlot.autosync.test",
		Root:        `/home/me/Backlot & Notes 100%`,
		ServiceName: "com.massivemoose.backlot.autosync.test.service",
		TimerName:   "com.massivemoose.backlot.autosync.test.timer",
		ServicePath: `/home/me/.config/systemd/user/com.massivemoose.backlot.autosync.test.service`,
		TimerPath:   `/home/me/.config/systemd/user/com.massivemoose.backlot.autosync.test.timer`,
	}
	config := Config{
		Binary:          `/opt/Backlot "Tools"/backlot`,
		ServiceName:     paths.ServiceName,
		TimerName:       paths.TimerName,
		IntervalSeconds: 900,
	}

	service, err := RenderSystemdService(paths, config)
	if err != nil {
		t.Fatalf("RenderSystemdService returned error: %v", err)
	}
	serviceText := string(service)
	for _, want := range []string{
		"[Unit]",
		"Description=Backlot auto-sync",
		"[Service]",
		"Type=oneshot",
		`ExecStart="/opt/Backlot \"Tools\"/backlot" "autosync" "run" "--root" "/home/me/Backlot & Notes 100%%"`,
	} {
		if !strings.Contains(serviceText, want) {
			t.Fatalf("systemd service missing %q:\n%s", want, serviceText)
		}
	}

	timer, err := RenderSystemdTimer(paths, config)
	if err != nil {
		t.Fatalf("RenderSystemdTimer returned error: %v", err)
	}
	timerText := string(timer)
	for _, want := range []string{
		"[Unit]",
		"Description=Run Backlot auto-sync",
		"[Timer]",
		"OnActiveSec=0",
		"OnUnitActiveSec=900s",
		"Persistent=false",
		"Unit=com.massivemoose.backlot.autosync.test.service",
		"[Install]",
		"WantedBy=timers.target",
	} {
		if !strings.Contains(timerText, want) {
			t.Fatalf("systemd timer missing %q:\n%s", want, timerText)
		}
	}
}
