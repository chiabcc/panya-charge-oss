package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/chiabcc/panya-charge-oss/internal/adapter/inbound/webui"
	"github.com/chiabcc/panya-charge-oss/internal/config"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	if err := run(*configPath); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run(configPath string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	level := slog.LevelInfo
	switch cfg.LogLevelUpper() {
	case "DEBUG":
		level = slog.LevelDebug
	case "WARN":
		level = slog.LevelWarn
	case "ERROR":
		level = slog.LevelError
	}
	var handler slog.Handler
	if cfg.Server.LogFormat == "json" {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	} else {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	}
	slog.SetDefault(slog.New(handler))

	facade, err := buildFacade(cfg)
	if err != nil {
		return fmt.Errorf("init csms: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	slog.Info("starting panya-charge-oss",
		"ocpp_port", cfg.Server.OCPPPort,
		"ocpp_path", cfg.Server.OCPPPath,
		"mqtt_broker", cfg.MQTT.Broker,
		"base_topic", cfg.MQTT.BaseTopic,
	)

	applier := newWebUIApplier(facade, ctx)

	if cfg.WebUI.Enabled || cfg.WebUI.StatusEnabled {
		isLoopback := isLoopback(cfg.WebUI.Listen)
		srv := webui.NewServer(configPath, cfg.WebUI.Listen, cfg.WebUI.Token, isLoopback, applier)
		if cfg.WebUI.StatusEnabled {
			srv.WithStatus(facade, cfg.Server.OCPPPort, cfg.Server.OCPPPath)
		}
		if cfg.WebUI.StatusEnabled && !cfg.WebUI.Enabled {
			srv.WithStatusOnly()
		}
		go func() {
			slog.Info("starting webui", "listen", cfg.WebUI.Listen, "config_enabled", cfg.WebUI.Enabled, "status_enabled", cfg.WebUI.StatusEnabled)
			if err := srv.Start(ctx); err != nil {
				slog.Warn("webui start failed", "error", err)
			}
		}()
	}

	go runFacade(facade, ctx)

	<-ctx.Done()
	applier.Shutdown()
	slog.Info("shutdown complete")
	return nil
}

func isLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr == "localhost" || addr == "::1"
	}
	return host == "127.0.0.1" || host == "::1" || host == "localhost"
}
