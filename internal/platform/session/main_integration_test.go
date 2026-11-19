//go:build integration

package session_test

import (
	"testing"

	"altalune.id/template/internal/testutil/pgtest"
)

func TestMain(m *testing.M) {
	pgtest.Main(m)
}
