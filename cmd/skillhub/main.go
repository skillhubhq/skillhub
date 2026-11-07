package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/skillhubhq/skillhub/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := cli.Execute(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr, cli.Options{}); err != nil {
		cli.PrintError(os.Stderr, err)
		os.Exit(1)
	}
}
