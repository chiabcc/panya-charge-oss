//go:build ignore

package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/chiabcc/panya-charge-oss/internal/adapter/inbound/webui"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	token := flag.Args()
	if len(token) == 0 {
		fmt.Fprintln(os.Stderr, "usage: webui-standalone -config <path> <token>")
		os.Exit(1)
	}
	tokenStr := token[0]

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	slog.Info("webui standalone server starting", "config", *configPath, "addr", "127.0.0.1:8888")
	srv := webui.NewServer(*configPath, "127.0.0.1:8888", tokenStr, true, nil)
	if err := srv.Start(ctx); err != nil {
		slog.Error("webui start failed", "error", err)
		os.Exit(1)
	}
}