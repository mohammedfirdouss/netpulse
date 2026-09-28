package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/mohammedfirdouss/netpulse/internal/config"
	"github.com/mohammedfirdouss/netpulse/internal/metrics"
	"github.com/mohammedfirdouss/netpulse/internal/probe"
	"github.com/mohammedfirdouss/netpulse/internal/scheduler"
)

func main() {
	cfgPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}

	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	m := metrics.New(reg)
	m.Init(cfg.Targets)

	probers := map[string]probe.Prober{
		"icmp": probe.ICMPProber{},
		"tcp":  probe.TCPProber{},
		"dns":  probe.DNSProber{},
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("ok"))
	})
	srv := &http.Server{Addr: cfg.ListenAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}

	// If the server can't bind, cancel ctx so the scheduler stops and main
	// exits through the normal path instead of os.Exit skipping cleanup.
	srvErr := make(chan error, 1)
	go func() {
		slog.Info("serving metrics", "addr", cfg.ListenAddr, "targets", len(cfg.Targets))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			srvErr <- err
			stop()
		}
	}()

	scheduler.Run(ctx, cfg.Targets, probers, m.Record) // blocks until SIGINT/SIGTERM

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(shutdownCtx)

	select {
	case err := <-srvErr:
		slog.Error("http server", "err", err)
		os.Exit(1)
	default:
		slog.Info("stopped")
	}
}
