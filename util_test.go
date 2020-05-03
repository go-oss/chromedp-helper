package helper

import (
	"strconv"
	"testing"
)

func pstrs(strs ...string) []*string {
	res := make([]*string, len(strs))
	for i := range strs {
		res[i] = &strs[i]
	}
	return res
}

func TestURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		endpoint string
		path     string
		val      []*string
		want     string
	}{
		{
			name:     "empty values",
			endpoint: "https://example.com",
			path:     "/path/to/resource",
			val:      nil,
			want:     "https://example.com/path/to/resource",
		},
		{
			name:     "with values",
			endpoint: "https://example.com",
			path:     "/path/%s/%s",
			val:      pstrs("to", "resource"),
			want:     "https://example.com/path/to/resource",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := URL(tt.endpoint, tt.path, tt.val...).String()
			if got != tt.want {
				t.Fatalf("%#v != %#v", got, tt.want)
			}
		})
	}
}

func TestFormat(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		format string
		val    []*string
		want   string
	}{
		{
			name:   "empty values",
			format: "empty values",
			val:    nil,
			want:   "empty values",
		},
		{
			name:   "single values",
			format: "single values %s",
			val:    pstrs("val1"),
			want:   "single values val1",
		},
		{
			name:   "multiple values",
			format: "multiple values %s, %s",
			val:    pstrs("val1", "val2"),
			want:   "multiple values val1, val2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := format(tt.format, tt.val...).String()
			if got != tt.want {
				t.Fatalf(`%#v != %#v`, got, tt.want)
			}
		})
	}
}

type testStringer int

func (s testStringer) String() string {
	return strconv.Itoa(int(s))
}

func TestToString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		value interface{}
		want  string
	}{
		{
			name:  "nil",
			value: nil,
			want:  "",
		},
		{
			name:  "string",
			value: "str",
			want:  "str",
		},
		{
			name: "string pointer",
			value: func() *string {
				str := "strp"
				return &str
			}(),
			want: "strp",
		},
		{
			name:  "fmt.Stringer implemented",
			value: testStringer(180),
			want:  "180",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toString(tt.value)
			if got != tt.want {
				t.Fatalf(`%#v != %#v`, got, tt.want)
			}
		})
	}
}
