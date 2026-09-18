package setup

import (
	"context"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ConnectMongo connects and pings within 10s.
func ConnectMongo(ctx context.Context, uri string) (*mongo.Client, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri).SetMaxPoolSize(100))
	if err != nil {
		return nil, err
	}
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, err
	}
	return client, nil
}

// TenantMongoDBName resolves the tenant database name:
// explicit override > (production && traya → TrayaProd) > "<tenant>_<suffix>".
func TenantMongoDBName(cfg *Config, tenantID string) string {
	if v, ok := cfg.MongoDBNameOverrides[strings.ToUpper(tenantID)]; ok && v != "" {
		return v
	}
	if cfg.IsProduction && tenantID == "traya" {
		return "TrayaProd"
	}
	return tenantID + "_" + cfg.MongoDBSuffix
}
