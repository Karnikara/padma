package webhook

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	kwebhook "github.com/karnikara/kanaka/webhook"

	"github.com/karnikara/padma/internal/platform/config"
)

// Run starts the webhook dispatcher: it drives the library's retry/backoff/
// dead-letter engine over the Postgres outbox and runs a background sweeper that
// re-arms deliveries stranded in 'delivering' by a crashed worker.
func Run(ctx context.Context, cfg config.Config, pool *pgxpool.Pool) error {
	store := NewStore(pool)
	disp := &kwebhook.Dispatcher{
		Store:        store,
		Sender:       &kwebhook.HTTPSender{Client: &http.Client{Timeout: 10 * time.Second}},
		Backoff:      expJitter(time.Second, 5*time.Minute),
		MaxAttempts:  cfg.WebhookMaxAttempts,
		Workers:      cfg.WebhookWorkers,
		PollInterval: time.Second,
	}

	go sweep(ctx, store, 30*time.Second)

	fmt.Printf("dispatcher: workers=%d max_attempts=%d\n", cfg.WebhookWorkers, cfg.WebhookMaxAttempts)
	return disp.Run(ctx)
}

// sweep periodically re-arms lease-expired 'delivering' rows until ctx ends.
func sweep(ctx context.Context, store *Store, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if n, err := store.Requeue(ctx); err == nil && n > 0 {
				fmt.Printf("dispatcher: re-armed %d stuck deliveries\n", n)
			}
		}
	}
}
