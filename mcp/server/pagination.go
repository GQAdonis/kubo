package server

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// defaultPageLimit is the contract's default Limit.
const defaultPageLimit = 100

// pageInfo is PageInfo in the contract's common definitions.
type pageInfo struct {
	HasMore    bool   `json:"has_more"`
	NextCursor string `json:"next_cursor,omitempty"`
}

// cursor is the content of an opaque pagination cursor. Clients never parse
// it; servers change it freely.
type cursor struct {
	// After is the sort key of the last item of the previous page.
	After string `json:"a,omitempty"`
	// Offset is the number of items already returned, for streams that have
	// no sort key.
	Offset int `json:"o,omitempty"`
}

func encodeCursor(c cursor) string {
	raw, _ := json.Marshal(c) // a struct of a string and an int cannot fail
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeCursor(s string) (cursor, error) {
	var c cursor
	if s == "" {
		return c, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return c, invalidArgument("cursor is not valid; use the next_cursor of a previous result")
	}
	if err := json.Unmarshal(raw, &c); err != nil || c.Offset < 0 {
		return cursor{}, invalidArgument("cursor is not valid; use the next_cursor of a previous result")
	}
	return c, nil
}

// pageLimit applies the default and rejects values outside the contract's
// range.
func pageLimit(n int) (int, error) {
	switch {
	case n == 0:
		return defaultPageLimit, nil
	case n < 0 || n > 1000:
		return 0, invalidArgument(fmt.Sprintf("limit must be between 1 and 1000, got %d", n))
	}
	return n, nil
}

// pageByKey returns the page of items (sorted ascending by key, keys unique)
// that follows the cursor.
func pageByKey[T any](items []T, key func(T) string, cur cursor, limit int) ([]T, pageInfo) {
	start := 0
	if cur.After != "" {
		for start < len(items) && key(items[start]) <= cur.After {
			start++
		}
	}
	end := min(start+limit, len(items))
	page := items[start:end]
	info := pageInfo{HasMore: end < len(items)}
	if info.HasMore && len(page) > 0 {
		info.NextCursor = encodeCursor(cursor{After: key(page[len(page)-1])})
	}
	return page, info
}
