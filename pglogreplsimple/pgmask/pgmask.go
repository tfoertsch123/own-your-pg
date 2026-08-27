package pgmask

import (
	"regexp"
	"strings"
	"net/url"
	"unicode"
	"fmt"
)

const masked = `MASKED`

var url_re = regexp.MustCompile(`^(postgres(?:ql)?://)(.*?):(.*?)@`)
func maskURL(s string) string {
	return url_re.ReplaceAllString(s, `${1}${2}:`+masked+`@`)
}

var cs_re = regexp.MustCompile(
	`(\bpassword\s*=\s*)(?:'(?:[^'\\]|\\.)*'|(?:[^'\\]|\\.)+)`,
)
func maskDSN(s string) string {
	return cs_re.ReplaceAllString(s, `${1}`+masked)
}

func MaskConnstr(s string) string {
	s = strings.TrimSpace(s)

	// looks like an URL
	if strings.HasPrefix(s, "postgres://") ||
		strings.HasPrefix(s, "postgresql://") {
		return maskURL(s)
	}

	return maskDSN(s)
}

func appendURL(s string, kv []string, override bool) (string, error) {
	u, err := url.Parse(s)
	if err != nil {return "", err}

	q := u.Query()
	for i := 0; i<len(kv); i += 2 {
		if override || ! q.Has(kv[i]) {q.Set(kv[i], kv[i+1])}
	}
	u.RawQuery = q.Encode()

	return u.String(), nil
}


func appendDSN(s string, kv []string, override bool) (string, error) {
	seen := make(map[string]string, len(kv)/2)
	for i := 0; i < len(kv); i += 2 {seen[kv[i]] = kv[i+1]}

	i := 0
	n := len(s)

	skipSpace := func() {
		for i < n && unicode.IsSpace(rune(s[i])) {i++}
	}

	readKey := func() (string, error) {
		start := i
		for i < n {
			c := s[i]
			if c == '=' || unicode.IsSpace(rune(c)) {
				break
			}
			i++
		}
		if start == i {
			return "", fmt.Errorf("expected key at pos %d", start)
		}
		return s[start:i], nil
	}

	readValue := func() (string, error) {
		if i >= n {
			return "", nil
		}
		start := i
		if s[i] == '\'' {
			i++					// consume opening quote
			for i < n {
				c := s[i]
				if c == '\'' {
					i++			// consume closing quote
					return s[start:i], nil
				}
				if c == '\\' {
					i++
					if i >= n {
						return "",
							fmt.Errorf("dangling escape at end of string")
					}
				}
				i++
			}
			return "", fmt.Errorf("unterminated quoted value")
		}

		for i < n && !unicode.IsSpace(rune(s[i])) {i++}
		return s[start:i], nil
	}

	var b strings.Builder
	joining := false
	for {
		skipSpace()
		if i >= n {
			break
		}

		k, err := readKey()
		if err != nil {
			return "", err
		}

		if i >= n || s[i] != '=' {
			return "", fmt.Errorf("expected '=' after key %q at pos %d", k, i)
		}
		i++ // '='

		v, err := readValue()
		if err != nil {
			return "", err
		}

		if _, ok := seen[k]; ok {
			if ! override {seen[k] = v}
		} else {
			if joining {
				b.WriteString(" ")
			} else {
				joining = true
			}
			b.WriteString(k)
			b.WriteString("=")
			b.WriteString(v)
		}
	}
	for i := 0; i<len(kv); i += 2 {
		if joining {
			b.WriteString(" ")
		} else {
			joining = true
		}
		b.WriteString(kv[i])
		b.WriteString("=")
		b.WriteString(seen[kv[i]])
	}

	return b.String(), nil
}

func AppendOptionOverride(s string, kv ...string) (string, error) {
	s = strings.TrimSpace(s)

	// looks like an URL
	if strings.HasPrefix(s, "postgres://") ||
		strings.HasPrefix(s, "postgresql://") {
		return appendURL(s, kv, true)
	}

	return appendDSN(s, kv, true)
}

func AppendOptionIfNotExists(s string, kv ...string) (string, error) {
	s = strings.TrimSpace(s)

	// looks like an URL
	if strings.HasPrefix(s, "postgres://") ||
		strings.HasPrefix(s, "postgresql://") {
		return appendURL(s, kv, false)
	}

	return appendDSN(s, kv, false)
}

// Local Variables:
// tab-width: 4
// End:
