// Command app is padma's single multi-mode binary. Subcommands:
//
//	app migrate      apply database migrations, then exit
//	app indexer      run the ingest.Runner + projector (#11)
//	app api          serve the query/webhook/merchant HTTP API
//	app dispatcher   run the webhook delivery loop (#10)
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/karnikara/kanaka/rpc"

	"github.com/karnikara/padma/internal/apiserver"
	"github.com/karnikara/padma/internal/indexer"
	"github.com/karnikara/padma/internal/merchant"
	"github.com/karnikara/padma/internal/platform/config"
	"github.com/karnikara/padma/internal/platform/db"
	"github.com/karnikara/padma/internal/webhook"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	if err := run(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, "app:", err)
		os.Exit(1)
	}
}

func run(mode string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// Signal-aware context so long-running modes shut down cleanly.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	switch mode {
	case "migrate":
		if err := db.Migrate(cfg.DatabaseURL); err != nil {
			return err
		}
		fmt.Println("migrations applied")
		return nil
	case "indexer":
		return runIndexer(ctx, cfg)
	case "api":
		return runAPI(ctx, cfg)
	case "dispatcher":
		return runDispatcher(ctx, cfg)
	case "merchant-key":
		return runProvisionMerchant(ctx, cfg)
	default:
		usage()
		return fmt.Errorf("unknown subcommand %q", mode)
	}
}

func runProvisionMerchant(ctx context.Context, cfg config.Config) error {
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	name := "merchant"
	if len(os.Args) > 2 {
		name = os.Args[2]
	}
	id, key, err := merchant.Provision(ctx, pool, name, "CKB")
	if err != nil {
		return err
	}
	fmt.Printf("merchant_id=%s\napi_key=%s\n", id, key)
	return nil
}

func runIndexer(ctx context.Context, cfg config.Config) error {
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	fmt.Println("indexer: starting (source:", cfg.IngestSource+")")
	if err := indexer.Run(ctx, cfg, pool); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	fmt.Println("indexer: done")
	return nil
}

func runDispatcher(ctx context.Context, cfg config.Config) error {
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := webhook.Run(ctx, cfg, pool); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	fmt.Println("dispatcher: stopped")
	return nil
}

func runAPI(ctx context.Context, cfg config.Config) error {
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	var fiberOpts []rpc.Option
	if cfg.FiberBiscuitToken != "" {
		fiberOpts = append(fiberOpts, rpc.WithAuthToken(cfg.FiberBiscuitToken))
	}
	fiber := rpc.NewClient(cfg.FiberRPCEndpoint, fiberOpts...)

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           apiserver.Router(pool, fiber),
		ReadHeaderTimeout: 10 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		fmt.Println("api: listening on", cfg.HTTPAddr)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: app <migrate|indexer|api|dispatcher>")
}
