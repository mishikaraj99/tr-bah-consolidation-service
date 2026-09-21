package common

// Redis keys this service shares with traya-api-server and traya-app-backend.
//
// Those services still write and read the UNPREFIXED names, so every shared key has two forms:
// the tenant-prefixed one this service writes, and the legacy one it must still honour until the
// Node services adopt the prefix. Reads try the prefixed key and fall back to the legacy key;
// invalidation deletes both. Once api-server and app-backend write prefixed keys, the legacy
// fallbacks simply stop matching and can be deleted along with LegacyRedisFallback.
const (
	// LoginStatusKeyFmt gates JWT auth. traya-api-server writes it at login.
	loginStatusPrefix = "user!"
	loginStatusSuffix = "login!status"
	// kitTrackerCalendarPrefix caches the v85 month calendar. traya-app-backend reads and writes it.
	kitTrackerCalendarPrefix = "kit-tracker-calendar!"
)

// LegacyRedisFallback controls whether reads fall back to the unprefixed key and whether
// invalidation also deletes it. Keep it true until api-server and app-backend write prefixed keys.
var LegacyRedisFallback = true

// TenantKey scopes a Redis key to a tenant: "traya:user!<id>login!status".
// An empty tenant yields the bare key, so a misconfigured caller cannot silently share a namespace.
func TenantKey(tenantID, key string) string {
	if tenantID == "" {
		return key
	}
	return tenantID + ":" + key
}

// LoginStatusKey is the tenant-scoped login gate key.
func LoginStatusKey(tenantID, userID string) string {
	return TenantKey(tenantID, LegacyLoginStatusKey(userID))
}

// LegacyLoginStatusKey is the unprefixed key traya-api-server writes today.
func LegacyLoginStatusKey(userID string) string {
	return loginStatusPrefix + userID + loginStatusSuffix
}

// KitTrackerCalendarKey is the tenant-scoped kit-tracker calendar cache key.
func KitTrackerCalendarKey(tenantID, userID string) string {
	return TenantKey(tenantID, LegacyKitTrackerCalendarKey(userID))
}

// LegacyKitTrackerCalendarKey is the unprefixed key traya-app-backend reads and writes today.
func LegacyKitTrackerCalendarKey(userID string) string {
	return kitTrackerCalendarPrefix + userID
}

// SharedKeyCandidates returns the keys a read should try, in order.
func SharedKeyCandidates(prefixed, legacy string) []string {
	if !LegacyRedisFallback || prefixed == legacy {
		return []string{prefixed}
	}
	return []string{prefixed, legacy}
}
