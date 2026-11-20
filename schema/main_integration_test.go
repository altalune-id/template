//go:build integration

package schema

import (
	"testing"

	"altalune.id/template/internal/testutil/pgtest"
)

func TestMain(m *testing.M) {
	pgtest.Main(m)
}
