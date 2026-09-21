package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTenantKey(t *testing.T) {
	assert.Equal(t, "traya:user!u1login!status", LoginStatusKey("traya", "u1"))
	assert.Equal(t, "user!u1login!status", LegacyLoginStatusKey("u1"))
	assert.Equal(t, "mool:kit-tracker-calendar!u1", KitTrackerCalendarKey("mool", "u1"))
	assert.Equal(t, "kit-tracker-calendar!u1", LegacyKitTrackerCalendarKey("u1"))
	assert.Equal(t, "bare", TenantKey("", "bare"), "an empty tenant must not invent a namespace")
}

func TestSharedKeyCandidates(t *testing.T) {
	defer func() { LegacyRedisFallback = true }()

	LegacyRedisFallback = true
	assert.Equal(t, []string{"traya:k", "k"}, SharedKeyCandidates("traya:k", "k"))

	LegacyRedisFallback = false
	assert.Equal(t, []string{"traya:k"}, SharedKeyCandidates("traya:k", "k"), "fallback disabled → prefixed only")

	LegacyRedisFallback = true
	assert.Equal(t, []string{"k"}, SharedKeyCandidates("k", "k"), "no duplicate when the tenant is empty")
}
