//go:build integration

package onboard_test

import (
	"testing"

	"altalune.id/template/internal/testutil/pgtest"
)

func runTests(m *testing.M) {
	pgtest.Main(m)
}
