package setup

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTenantMongoDBName(t *testing.T) {
	cfg := &Config{MongoDBSuffix: "dev", MongoDBNameOverrides: map[string]string{"ACNE": "acne_custom"}}
	assert.Equal(t, "mool_dev", TenantMongoDBName(cfg, "mool"))
	assert.Equal(t, "acne_custom", TenantMongoDBName(cfg, "acne"))
	cfg.IsProduction = true
	assert.Equal(t, "TrayaProd", TenantMongoDBName(cfg, "traya"))
	assert.Equal(t, "mool_dev", TenantMongoDBName(cfg, "mool"))
}

func TestConnectMongo_Integration(t *testing.T) {
	uri := os.Getenv("TEST_MONGO_URI")
	if uri == "" {
		t.Skip("TEST_MONGO_URI not set")
	}
	c, err := ConnectMongo(context.Background(), uri)
	require.NoError(t, err)
	_ = c.Disconnect(context.Background())
}
