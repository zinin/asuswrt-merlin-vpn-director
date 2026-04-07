package updater

import (
	"strings"
	"testing"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Version
		wantErr bool
	}{
		{
			name:  "valid with v prefix",
			input: "v1.2.3",
			want:  Version{Major: 1, Minor: 2, Patch: 3, Raw: "v1.2.3"},
		},
		{
			name:  "valid without v prefix",
			input: "1.2.3",
			want:  Version{Major: 1, Minor: 2, Patch: 3, Raw: "1.2.3"},
		},
		{
			name:  "all zeros",
			input: "v0.0.0",
			want:  Version{Major: 0, Minor: 0, Patch: 0, Raw: "v0.0.0"},
		},
		{
			name:  "large numbers",
			input: "v10.20.30",
			want:  Version{Major: 10, Minor: 20, Patch: 30, Raw: "v10.20.30"},
		},
		{
			name:    "dev version",
			input:   "dev",
			wantErr: true,
		},
		{
			name:    "pre-release version",
			input:   "v1.2.3-rc1",
			wantErr: true,
		},
		{
			name:    "pre-release beta",
			input:   "v1.2.3-beta.1",
			wantErr: true,
		},
		{
			name:    "incomplete version two parts",
			input:   "v1.2",
			wantErr: true,
		},
		{
			name:    "incomplete version one part",
			input:   "v1",
			wantErr: true,
		},
		{
			name:    "invalid format",
			input:   "invalid",
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "non-numeric major",
			input:   "vX.2.3",
			wantErr: true,
		},
		{
			name:    "non-numeric minor",
			input:   "v1.Y.3",
			wantErr: true,
		},
		{
			name:    "non-numeric patch",
			input:   "v1.2.Z",
			wantErr: true,
		},
		{
			name:    "negative number",
			input:   "v-1.2.3",
			wantErr: true,
		},
		{
			name:    "extra parts",
			input:   "v1.2.3.4",
			wantErr: true,
		},
		{
			name:    "whitespace",
			input:   " v1.2.3 ",
			wantErr: true,
		},
		{
			name:    "newline in version",
			input:   "v1.2.3\n",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseVersion(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseVersion(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if got.Major != tt.want.Major || got.Minor != tt.want.Minor || got.Patch != tt.want.Patch || got.Raw != tt.want.Raw {
					t.Errorf("ParseVersion(%q) = %+v, want %+v", tt.input, got, tt.want)
				}
			}
		})
	}
}

func TestVersionCompare(t *testing.T) {
	tests := []struct {
		name string
		v1   Version
		v2   Version
		want int
	}{
		{
			name: "equal versions",
			v1:   Version{Major: 1, Minor: 2, Patch: 3},
			v2:   Version{Major: 1, Minor: 2, Patch: 3},
			want: 0,
		},
		{
			name: "major greater",
			v1:   Version{Major: 2, Minor: 0, Patch: 0},
			v2:   Version{Major: 1, Minor: 9, Patch: 9},
			want: 1,
		},
		{
			name: "major less",
			v1:   Version{Major: 1, Minor: 9, Patch: 9},
			v2:   Version{Major: 2, Minor: 0, Patch: 0},
			want: -1,
		},
		{
			name: "minor greater",
			v1:   Version{Major: 1, Minor: 3, Patch: 0},
			v2:   Version{Major: 1, Minor: 2, Patch: 9},
			want: 1,
		},
		{
			name: "minor less",
			v1:   Version{Major: 1, Minor: 2, Patch: 9},
			v2:   Version{Major: 1, Minor: 3, Patch: 0},
			want: -1,
		},
		{
			name: "patch greater",
			v1:   Version{Major: 1, Minor: 2, Patch: 4},
			v2:   Version{Major: 1, Minor: 2, Patch: 3},
			want: 1,
		},
		{
			name: "patch less",
			v1:   Version{Major: 1, Minor: 2, Patch: 3},
			v2:   Version{Major: 1, Minor: 2, Patch: 4},
			want: -1,
		},
		{
			name: "zero versions equal",
			v1:   Version{Major: 0, Minor: 0, Patch: 0},
			v2:   Version{Major: 0, Minor: 0, Patch: 0},
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.v1.Compare(tt.v2)
			if got != tt.want {
				t.Errorf("(%+v).Compare(%+v) = %d, want %d", tt.v1, tt.v2, got, tt.want)
			}
		})
	}
}

func TestVersionIsOlderThan(t *testing.T) {
	tests := []struct {
		name string
		v1   Version
		v2   Version
		want bool
	}{
		{
			name: "older version",
			v1:   Version{Major: 1, Minor: 0, Patch: 0},
			v2:   Version{Major: 2, Minor: 0, Patch: 0},
			want: true,
		},
		{
			name: "equal version",
			v1:   Version{Major: 1, Minor: 2, Patch: 3},
			v2:   Version{Major: 1, Minor: 2, Patch: 3},
			want: false,
		},
		{
			name: "newer version",
			v1:   Version{Major: 2, Minor: 0, Patch: 0},
			v2:   Version{Major: 1, Minor: 0, Patch: 0},
			want: false,
		},
		{
			name: "older by minor",
			v1:   Version{Major: 1, Minor: 1, Patch: 0},
			v2:   Version{Major: 1, Minor: 2, Patch: 0},
			want: true,
		},
		{
			name: "older by patch",
			v1:   Version{Major: 1, Minor: 2, Patch: 2},
			v2:   Version{Major: 1, Minor: 2, Patch: 3},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.v1.IsOlderThan(tt.v2)
			if got != tt.want {
				t.Errorf("(%+v).IsOlderThan(%+v) = %v, want %v", tt.v1, tt.v2, got, tt.want)
			}
		})
	}
}

func TestIsValidVersion(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name:  "valid with v prefix",
			input: "v1.2.3",
			want:  true,
		},
		{
			name:  "valid without v prefix",
			input: "1.2.3",
			want:  true,
		},
		{
			name:  "valid with dash pre-release",
			input: "v1.2.3-rc1",
			want:  true,
		},
		{
			name:  "valid with multiple dashes",
			input: "v1.2.3-beta-1",
			want:  true,
		},
		{
			name:  "empty string",
			input: "",
			want:  false,
		},
		{
			name:  "shell injection semicolon",
			input: "v1.2.3;rm -rf /",
			want:  false,
		},
		{
			name:  "shell injection command substitution",
			input: "v1.2.3$(whoami)",
			want:  false,
		},
		{
			name:  "shell injection backticks",
			input: "v1.2.3`id`",
			want:  false,
		},
		{
			name:  "shell injection double quote",
			input: "v1.2.3\"test",
			want:  false,
		},
		{
			name:  "shell injection single quote",
			input: "v1.2.3'test",
			want:  false,
		},
		{
			name:  "shell injection pipe",
			input: "v1.2.3|cat /etc/passwd",
			want:  false,
		},
		{
			name:  "shell injection ampersand",
			input: "v1.2.3&id",
			want:  false,
		},
		{
			name:  "shell injection newline",
			input: "v1.2.3\nid",
			want:  false,
		},
		{
			name:  "shell injection carriage return",
			input: "v1.2.3\rid",
			want:  false,
		},
		{
			name:  "too long version",
			input: strings.Repeat("v", 51),
			want:  false,
		},
		{
			name:  "max length version",
			input: strings.Repeat("1", 50),
			want:  true,
		},
		{
			name:  "space in version",
			input: "v1.2.3 extra",
			want:  false,
		},
		{
			name:  "tab in version",
			input: "v1.2.3\textra",
			want:  false,
		},
		{
			name:  "slash in version",
			input: "v1.2.3/test",
			want:  false,
		},
		{
			name:  "backslash in version",
			input: "v1.2.3\\test",
			want:  false,
		},
		{
			name:  "angle brackets",
			input: "v1.2.3<test>",
			want:  false,
		},
		{
			name:  "curly braces",
			input: "v1.2.3{test}",
			want:  false,
		},
		{
			name:  "square brackets",
			input: "v1.2.3[test]",
			want:  false,
		},
		{
			name:  "equals sign",
			input: "v1.2.3=test",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsValidVersion(tt.input)
			if got != tt.want {
				t.Errorf("IsValidVersion(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
