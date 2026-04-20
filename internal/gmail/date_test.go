package gmail

import (
	"testing"
)

func TestParseQuarterQuery(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		// ── Basic quarter expansion ──────────────────────────────────────────
		{
			name:  "Q1 expands to Jan-Mar",
			input: "Q1 2026",
			want:  "after:2026/01/01 before:2026/04/01",
		},
		{
			name:  "Q2 expands to Apr-Jun",
			input: "Q2 2026",
			want:  "after:2026/04/01 before:2026/07/01",
		},
		{
			name:  "Q3 expands to Jul-Sep",
			input: "Q3 2026",
			want:  "after:2026/07/01 before:2026/10/01",
		},
		{
			name:  "Q4 expands to Oct-Dec",
			input: "Q4 2026",
			want:  "after:2026/10/01 before:2027/01/01",
		},
		// ── Year boundary ────────────────────────────────────────────────────
		{
			name:  "Q4 wraps year correctly",
			input: "Q4 2025",
			want:  "after:2025/10/01 before:2026/01/01",
		},
		{
			name:  "Q1 different year",
			input: "Q1 2024",
			want:  "after:2024/01/01 before:2024/04/01",
		},
		// ── Case insensitivity ───────────────────────────────────────────────
		{
			name:  "lowercase q1",
			input: "q1 2026",
			want:  "after:2026/01/01 before:2026/04/01",
		},
		{
			name:  "mixed case q3",
			input: "q3 2025",
			want:  "after:2025/07/01 before:2025/10/01",
		},
		// ── Combined with Gmail operators ────────────────────────────────────
		{
			name:  "quarter combined with from",
			input: "from:boss@company.com Q1 2026",
			want:  "from:boss@company.com after:2026/01/01 before:2026/04/01",
		},
		{
			name:  "quarter at beginning",
			input: "Q2 2025 is:unread",
			want:  "after:2025/04/01 before:2025/07/01 is:unread",
		},
		{
			name:  "quarter in middle",
			input: "from:me Q3 2025 has:attachment",
			want:  "from:me after:2025/07/01 before:2025/10/01 has:attachment",
		},
		{
			name:  "multiple quarters in one query",
			input: "Q1 2025 OR Q2 2025",
			want:  "after:2025/01/01 before:2025/04/01 OR after:2025/04/01 before:2025/07/01",
		},
		// ── No quarter — query is passed through unchanged ───────────────────
		{
			name:  "no quarter keeps query unchanged",
			input: "from:example@gmail.com is:unread",
			want:  "from:example@gmail.com is:unread",
		},
		{
			name:  "explicit date operators are unchanged",
			input: "after:2026/01/01 before:2026/04/01",
			want:  "after:2026/01/01 before:2026/04/01",
		},
		{
			name:  "empty query is unchanged",
			input: "",
			want:  "",
		},
		// ── Edge cases — must NOT match these ───────────────────────────────
		{
			name:  "Q5 is not a valid quarter, left unchanged",
			input: "Q5 2026",
			want:  "Q5 2026",
		},
		{
			name:  "Q0 is not a valid quarter, left unchanged",
			input: "Q0 2026",
			want:  "Q0 2026",
		},
		{
			name:  "subject containing 'Q1' inside word is unchanged",
			input: "subject:Q1Report",
			want:  "subject:Q1Report",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseQuarterQuery(tt.input)
			if got != tt.want {
				t.Errorf("parseQuarterQuery(%q)\n  got  %q\n  want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestQuarterStartMonth(t *testing.T) {
	tests := []struct{ q, want int }{
		{1, 1},
		{2, 4},
		{3, 7},
		{4, 10},
	}
	for _, tt := range tests {
		got := quarterStartMonth(tt.q)
		if got != tt.want {
			t.Errorf("quarterStartMonth(%d) = %d, want %d", tt.q, got, tt.want)
		}
	}
}
