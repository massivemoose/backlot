package autosync

import (
	"bytes"
	"fmt"
)

func RenderSystemdService(paths Paths, config Config) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString("[Unit]\n")
	b.WriteString("Description=Backlot auto-sync\n")
	b.WriteByte('\n')
	b.WriteString("[Service]\n")
	b.WriteString("Type=oneshot\n")
	b.WriteString("ExecStart=")
	writeSystemdExec(&b, config.Binary, "autosync", "run", "--root", paths.Root)
	b.WriteByte('\n')
	return b.Bytes(), nil
}

func RenderSystemdTimer(paths Paths, config Config) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString("[Unit]\n")
	b.WriteString("Description=Run Backlot auto-sync\n")
	b.WriteByte('\n')
	b.WriteString("[Timer]\n")
	b.WriteString("OnActiveSec=0\n")
	b.WriteString(fmt.Sprintf("OnUnitActiveSec=%ds\n", config.IntervalSeconds))
	b.WriteString("Persistent=false\n")
	b.WriteString("Unit=")
	b.WriteString(escapeSystemdValue(config.ServiceName))
	b.WriteByte('\n')
	b.WriteByte('\n')
	b.WriteString("[Install]\n")
	b.WriteString("WantedBy=timers.target\n")
	return b.Bytes(), nil
}

func writeSystemdExec(b *bytes.Buffer, args ...string) {
	for i, arg := range args {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(quoteSystemdExecArg(arg))
	}
}

func quoteSystemdExecArg(arg string) string {
	var b bytes.Buffer
	b.WriteByte('"')
	for _, r := range arg {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '%':
			b.WriteString(`%%`)
		case '$':
			b.WriteString(`$$`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func escapeSystemdValue(value string) string {
	var b bytes.Buffer
	for _, r := range value {
		if r == '%' {
			b.WriteString("%%")
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
