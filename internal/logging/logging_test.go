package logging

import "testing"

func TestQuoteValueProducesSingleLineValues(t *testing.T) {
	if got := quoteValue("a line\nb line"); got != `"a line\nb line"` {
		t.Fatalf("quoteValue() = %q", got)
	}
	if got := quoteValue("/api/v1/robot"); got != "/api/v1/robot" {
		t.Fatalf("safe value = %q", got)
	}
}

func TestColourForHTTPStatus(t *testing.T) {
	if got := colourFor("[INFO] [status=200]"); got != ansiGreen {
		t.Fatalf("success colour = %q", got)
	}
	if got := colourFor("[ERROR] [status=500]"); got != ansiRed {
		t.Fatalf("failure colour = %q", got)
	}
}

func TestQuoteValueKeepsObjectsStructured(t *testing.T) {
	if got := quoteValue(map[string]any{"page": 1, "root": "/tmp/project"}); got != `{"page":1,"root":"/tmp/project"}` {
		t.Fatalf("object value = %q", got)
	}
	if got := quoteValue(RawJSON(`{"error":"not found"}`)); got != `{"error":"not found"}` {
		t.Fatalf("raw JSON value = %q", got)
	}
}
