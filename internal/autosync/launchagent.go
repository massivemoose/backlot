package autosync

import (
	"bytes"
	"encoding/xml"
	"fmt"
)

func RenderLaunchAgent(paths Paths, config Config) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString(xml.Header)
	b.WriteString(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">`)
	b.WriteByte('\n')
	b.WriteString(`<plist version="1.0"><dict>`)
	writePlistString(&b, "Label", paths.Label)
	b.WriteString(`<key>ProgramArguments</key><array>`)
	for _, arg := range []string{config.Binary, "autosync", "run", "--root", paths.Root} {
		b.WriteString("<string>")
		if err := xml.EscapeText(&b, []byte(arg)); err != nil {
			return nil, err
		}
		b.WriteString("</string>")
	}
	b.WriteString(`</array>`)
	b.WriteString(`<key>RunAtLoad</key><true></true>`)
	b.WriteString(fmt.Sprintf(`<key>StartInterval</key><integer>%d</integer>`, config.IntervalSeconds))
	writePlistString(&b, "ProcessType", "Background")
	b.WriteString(`</dict></plist>`)
	b.WriteByte('\n')
	return b.Bytes(), nil
}

func writePlistString(b *bytes.Buffer, key, value string) {
	b.WriteString("<key>")
	_ = xml.EscapeText(b, []byte(key))
	b.WriteString("</key><string>")
	_ = xml.EscapeText(b, []byte(value))
	b.WriteString("</string>")
}
