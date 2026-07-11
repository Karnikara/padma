package merchant

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/karnikara/padma/internal/platform/auth"
	"github.com/karnikara/padma/internal/platform/id"
)

// Provision creates a merchant and returns its id and a freshly generated API key
// (shown once — only the hash is stored). Merchants are provisioned out of band,
// not via the public API.
func Provision(ctx context.Context, pool *pgxpool.Pool, name, settlementAsset string) (merchantID, apiKey string, err error) {
	merchantID = id.New()
	apiKey, hash := auth.NewAPIKey()
	if settlementAsset == "" {
		settlementAsset = "CKB"
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO merchants (id, name, api_key_hash, settlement_asset)
		VALUES ($1, $2, $3, $4)`, merchantID, name, hash, settlementAsset)
	if err != nil {
		return "", "", fmt.Errorf("merchant: provision: %w", err)
	}
	return merchantID, apiKey, nil
}
