// Package logging provides compact, colour-aware structured console logging.
package logging

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
)

// Level controls both the semantic label and the terminal colour of an entry.
type Level string

const (
	Debug Level = "DEBUG"
	Info  Level = "INFO"
	Warn  Level = "WARN"
	Error Level = "ERROR"
)

// Fields holds machine-readable metadata associated with one log event.
type Fields map[string]any

// RawJSON marks an already-validated JSON value which should remain an object
// or array in a structured console entry instead of being quoted as a string.
type RawJSON string

const (
	ansiReset  = "\x1b[0m"
	ansiGray   = "\x1b[90m"
	ansiBlue   = "\x1b[36m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiRed    = "\x1b[31m"
)

// ConfigureStandardLogger keeps existing log.Printf callers useful while
// making their terminal output readable. Colours are disabled for redirected
// service logs unless ALX_LOG_COLOR=always explicitly opts in.
func ConfigureStandardLogger(output io.Writer) {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	log.SetOutput(colourWriter{output: output, enabled: colourEnabled(output)})
}

// Event writes one stable bracketed line. Fields are sorted so both people and
// log collectors see a consistent schema across requests without losing the
// familiar, easy-to-scan console style.
func Event(level Level, name string, fields Fields) {
	parts := []string{"[" + string(level) + "]", "[" + name + "]"}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		parts = append(parts, "["+key+"="+quoteValue(fields[key])+"]")
	}
	log.Print(strings.Join(parts, " "))
}

func InfoEvent(name string, fields Fields)  { Event(Info, name, fields) }
func WarnEvent(name string, fields Fields)  { Event(Warn, name, fields) }
func ErrorEvent(name string, fields Fields) { Event(Error, name, fields) }

func quoteValue(value any) string {
	switch item := value.(type) {
	case nil:
		return "null"
	case string:
		if item != "" && !strings.ContainsAny(item, " \t\r\n=\"\\") {
			return item
		}
		return strconv.Quote(item)
	case fmt.Stringer:
		return quoteValue(item.String())
	case bool:
		return strconv.FormatBool(item)
	case int:
		return strconv.Itoa(item)
	case int64:
		return strconv.FormatInt(item, 10)
	case uint64:
		return strconv.FormatUint(item, 10)
	case float64:
		return strconv.FormatFloat(item, 'f', -1, 64)
	case RawJSON:
		if json.Valid([]byte(item)) {
			return string(item)
		}
		return quoteValue(string(item))
	default:
		encoded, err := json.Marshal(item)
		if err == nil {
			return string(encoded)
		}
		return strconv.Quote(fmt.Sprint(item))
	}
}

type colourWriter struct {
	output  io.Writer
	enabled bool
}

func (w colourWriter) Write(data []byte) (int, error) {
	if !w.enabled {
		return w.output.Write(data)
	}
	colour := colourFor(string(data))
	if colour == "" {
		return w.output.Write(data)
	}
	_, err := io.WriteString(w.output, colour+string(data)+ansiReset)
	if err != nil {
		return 0, err
	}
	return len(data), nil
}

func colourFor(entry string) string {
	switch {
	case strings.Contains(entry, "[ERROR]") || strings.Contains(entry, "失败") || strings.Contains(strings.ToLower(entry), "error"):
		return ansiRed
	case strings.Contains(entry, "[WARN]") || strings.Contains(entry, "不可用") || strings.Contains(entry, "暂停"):
		return ansiYellow
	case strings.Contains(entry, "[status=2") || strings.Contains(entry, "[INFO]"):
		return ansiGreen
	case strings.Contains(entry, "[DEBUG]"):
		return ansiGray
	default:
		return ansiBlue
	}
}

func colourEnabled(output io.Writer) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("ALX_LOG_COLOR"))) {
	case "always", "true", "1":
		return true
	case "never", "false", "0":
		return false
	}
	if os.Getenv("NO_COLOR") != "" || strings.EqualFold(os.Getenv("TERM"), "dumb") {
		return false
	}
	file, ok := output.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
