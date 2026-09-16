package sqlite

import "github.com/go-jet/jet/v2/sqlite"

// NOTE: wrapping jet's shared NULL singleton races; CAST builds a fresh expression instead.

// NullText returns a fresh SQL NULL typed as TEXT.
func NullText() sqlite.StringExpression {
	return sqlite.CAST(sqlite.NULL).AS_TEXT()
}
