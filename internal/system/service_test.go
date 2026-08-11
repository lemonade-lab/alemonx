package system

import "testing"

func TestHasPortArgument(t *testing.T) {
	tests := []struct {
		name      string
		arguments []string
		want      bool
	}{
		{name: "separate value", arguments: []string{"serve", "--port", "17390"}, want: true},
		{name: "equals value", arguments: []string{"--port=17390"}, want: true},
		{name: "missing value", arguments: []string{"serve", "--port"}, want: false},
		{name: "not configured", arguments: []string{"serve", "--host", "127.0.0.1"}, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := hasPortArgument(test.arguments); got != test.want {
				t.Fatalf("hasPortArgument(%q) = %t, want %t", test.arguments, got, test.want)
			}
		})
	}
}
