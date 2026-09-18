package setup

import (
	"crypto/tls"

	"github.com/redis/go-redis/v9"
)

// ConnectRedis returns a client for the shared services cache.
func ConnectRedis(cfg *Config) *redis.Client {
	opt := &redis.Options{Addr: cfg.RedisAddr, Username: cfg.RedisUsername, Password: cfg.RedisPassword, MaxRetries: 3}
	if cfg.RedisTLS {
		opt.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	return redis.NewClient(opt)
}
