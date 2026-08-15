package web

import (
	_ "embed"
	"encoding/json"
	"sort"
	"strings"
)

//go:embed alemonjs_theme.json
var alemonjsThemeJSON []byte

// alemonjsThemeStyleTag is the <style> block injected into every webview
// (system plugin pages, robot app pages and the robot app proxy). It turns the
// alemonjs-* variable contract from docs/theme.json into real CSS custom
// properties, so pages can rely on var(--alemonjs-*) without shipping their
// own copy of the palette.
var alemonjsThemeStyleTag string

func init() {
	var variables map[string]string
	if err := json.Unmarshal(alemonjsThemeJSON, &variables); err != nil {
		panic("alemonjs_theme.json: " + err.Error())
	}
	keys := make([]string, 0, len(variables))
	for key := range variables {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var builder strings.Builder
	builder.WriteString("<style data-alx-alemonjs-theme>\n:root {\n")
	for _, key := range keys {
		builder.WriteString("  --")
		builder.WriteString(key)
		builder.WriteString(": ")
		builder.WriteString(variables[key])
		builder.WriteString(";\n")
	}
	builder.WriteString("}\n[data-theme='dark'] {\n")
	for _, key := range keys {
		if !strings.HasPrefix(key, "alemonjs-dark-") {
			continue
		}
		builder.WriteString("  --alemonjs-")
		builder.WriteString(strings.TrimPrefix(key, "alemonjs-dark-"))
		builder.WriteString(": ")
		builder.WriteString(variables[key])
		builder.WriteString(";\n")
	}
	builder.WriteString("}\n</style>")
	alemonjsThemeStyleTag = builder.String()
}
