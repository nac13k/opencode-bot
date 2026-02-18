package opencode

import "testing"

func TestParseCLISessionTitleAndUpdated(t *testing.T) {
	tests := []struct {
		name        string
		line        string
		sessionID   string
		wantTitle   string
		wantUpdated string
	}{
		{
			name:        "time only updated",
			line:        "ses_39b217c8affe9GAKVHVOQ3zLa0  Comandos integrables con API de opencode                            6:03 PM",
			sessionID:   "ses_39b217c8affe9GAKVHVOQ3zLa0",
			wantTitle:   "Comandos integrables con API de opencode",
			wantUpdated: "6:03 PM",
		},
		{
			name:        "time and date updated",
			line:        "ses_39b296c7effec1pavym6BucU4F  Saludo informal / Consulta rápida                                   11:11 PM · 2/15/2026",
			sessionID:   "ses_39b296c7effec1pavym6BucU4F",
			wantTitle:   "Saludo informal / Consulta rápida",
			wantUpdated: "11:11 PM · 2/15/2026",
		},
		{
			name:        "no updated column",
			line:        "ses_abc123  Titulo sin fecha",
			sessionID:   "ses_abc123",
			wantTitle:   "Titulo sin fecha",
			wantUpdated: "",
		},
		{
			name:        "empty remainder",
			line:        "ses_xyz999",
			sessionID:   "ses_xyz999",
			wantTitle:   "(untitled)",
			wantUpdated: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			title, updated := parseCLISessionTitleAndUpdated(tt.line, tt.sessionID)
			if title != tt.wantTitle {
				t.Fatalf("title mismatch: got %q want %q", title, tt.wantTitle)
			}
			if updated != tt.wantUpdated {
				t.Fatalf("updated mismatch: got %q want %q", updated, tt.wantUpdated)
			}
		})
	}
}

func TestParseTimestamp(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{name: "unix seconds", value: "1739714400"},
		{name: "unix millis", value: "1739714400000"},
		{name: "unix micros", value: "1739714400000000"},
		{name: "unix nanos", value: "1739714400000000000"},
		{name: "rfc3339", value: "2026-02-15T23:11:00Z"},
		{name: "cli time", value: "6:03 PM"},
		{name: "cli time date", value: "11:11 PM · 2/15/2026"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseTimestamp(tt.value); got <= 0 {
				t.Fatalf("expected parsed timestamp > 0, got %d for %q", got, tt.value)
			}
		})
	}

	if got := parseTimestamp("not-a-date"); got != 0 {
		t.Fatalf("expected invalid timestamp to parse as 0, got %d", got)
	}
}

func TestNormalizeUnixMillis(t *testing.T) {
	tests := []struct {
		name string
		in   int64
		want int64
	}{
		{name: "seconds to millis", in: 1739714400, want: 1739714400000},
		{name: "millis unchanged", in: 1739714400000, want: 1739714400000},
		{name: "micros to millis", in: 1739714400000000, want: 1739714400000},
		{name: "nanos to millis", in: 1739714400000000000, want: 1739714400000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeUnixMillis(tt.in); got != tt.want {
				t.Fatalf("normalizeUnixMillis mismatch: got %d want %d", got, tt.want)
			}
		})
	}
}
