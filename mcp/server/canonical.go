package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
)

// canonicalJSON encodes a decoded JSON value (decoded with UseNumber) in the
// RFC 8785 canonical form: sorted object keys, no insignificant whitespace,
// and numbers in their shortest form. Confirm tokens bind to its SHA-256, so
// key order and number spelling do not change what a confirmation covers.
func canonicalJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	if err := writeCanonical(&buf, v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeCanonical(buf *bytes.Buffer, v any) error {
	switch x := v.(type) {
	case nil:
		buf.WriteString("null")
	case bool:
		buf.WriteString(strconv.FormatBool(x))
	case string:
		return writeCanonicalString(buf, x)
	case json.Number:
		s, err := canonicalNumber(x)
		if err != nil {
			return err
		}
		buf.WriteString(s)
	case []any:
		buf.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := writeCanonical(buf, e); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		buf.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := writeCanonicalString(buf, k); err != nil {
				return err
			}
			buf.WriteByte(':')
			if err := writeCanonical(buf, x[k]); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
	default:
		return fmt.Errorf("canonical JSON: unsupported value of type %T", v)
	}
	return nil
}

func writeCanonicalString(buf *bytes.Buffer, s string) error {
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		return fmt.Errorf("canonical JSON: %w", err)
	}
	// Encode appends a newline.
	buf.Truncate(buf.Len() - 1)
	return nil
}

// maxExactInt is the largest integer a float64 holds exactly.
const maxExactInt = 1 << 53

// canonicalNumber spells a number the way ECMAScript does. Integer literals
// are kept exact so two different large integers never collapse to one.
func canonicalNumber(n json.Number) (string, error) {
	s := n.String()
	if !strings.ContainsAny(s, ".eE") {
		if s == "-0" {
			return "0", nil
		}
		return s, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return "", fmt.Errorf("canonical JSON: number %q: %w", s, err)
	}
	if math.IsInf(f, 0) || math.IsNaN(f) {
		return "", fmt.Errorf("canonical JSON: number %q is not finite", s)
	}
	if f == 0 {
		return "0", nil
	}
	if f == math.Trunc(f) && math.Abs(f) < maxExactInt {
		return strconv.FormatInt(int64(f), 10), nil
	}
	abs := math.Abs(f)
	if abs >= 1e21 || abs < 1e-6 {
		out := strconv.FormatFloat(f, 'e', -1, 64)
		mant, exp, _ := strings.Cut(out, "e")
		sign := exp[:1]
		digits := strings.TrimLeft(exp[1:], "0")
		return mant + "e" + sign + digits, nil
	}
	return strconv.FormatFloat(f, 'f', -1, 64), nil
}

// argsDigest returns the hex SHA-256 of the canonical JSON of args without
// the confirm_token field, and the token that was present ("" if none).
func argsDigest(raw json.RawMessage) (digest, token string, args map[string]any, err error) {
	if len(raw) == 0 || string(raw) == "null" {
		raw = json.RawMessage("{}")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&args); err != nil {
		return "", "", nil, fmt.Errorf("decode arguments: %w", err)
	}
	if t, ok := args["confirm_token"].(string); ok {
		token = t
	}
	delete(args, "confirm_token")
	canon, err := canonicalJSON(args)
	if err != nil {
		return "", "", nil, err
	}
	sum := sha256.Sum256(canon)
	return hex.EncodeToString(sum[:]), token, args, nil
}
