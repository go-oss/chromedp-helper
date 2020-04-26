package helper

import (
	"fmt"
	"net/url"
)

// URL returns url string from endpoint and path.
func URL(endpoint, path string, val ...*string) Stringer {
	u, err := url.Parse(endpoint)
	if err != nil {
		panic(err)
	}

	return func() string {
		if len(val) == 0 {
			u.Path = path
			return u.String()
		}

		u.Path = format(path, val...).String()
		return u.String()
	}
}

func format(format string, val ...*string) Stringer {
	return func() string {
		vs := make([]interface{}, 0, len(val))
		for _, v := range val {
			vs = append(vs, *v)
		}
		return fmt.Sprintf(format, vs...)
	}
}

// Stringer implements fmt.Stringer.
type Stringer func() string

func (s Stringer) String() string {
	return s()
}

func toString(val interface{}) string {
	switch v := val.(type) {
	case string:
		return v

	case fmt.Stringer:
		return v.String()

	default:
		return ""
	}
}
