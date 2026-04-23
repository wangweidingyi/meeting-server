package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"meeting-server/internal/app"
	"meeting-server/internal/config"
)

func main() {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	application := app.NewFromConfig(cfg)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	fmt.Printf("meeting-server starting (%s)\n", cfg.Summary())

	if err := application.Run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
