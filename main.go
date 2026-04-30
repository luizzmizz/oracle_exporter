package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/luizzmizz/oracle_exporter/internal/collector"
	"github.com/luizzmizz/oracle_exporter/internal/config"
	"github.com/luizzmizz/oracle_exporter/internal/target"
)

func main() {
	var (
		configFile = flag.String("config", "config.yaml", "Path to config file")
		listenAddr = flag.String("web.listen-address", ":9161", "Address to listen on for metrics")
		logLevel   = flag.String("log.level", "info", "Log level: debug, info, warn, error")
	)
	flag.Parse()

	var level slog.Level
	if err := level.UnmarshalText([]byte(*logLevel)); err != nil {
		level = slog.LevelInfo
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)

	cfg, err := config.Load(*configFile)
	if err != nil {
		slog.Error("failed to load config", "err", err)
		os.Exit(1)
	}

	// Build all target entries from config immediately — targets exist regardless
	// of whether the DB is reachable right now.
	targets := make(map[string]*targetEntry, len(cfg.Targets))
	for name, tcfg := range cfg.Targets {
		targets[name] = &targetEntry{
			name:   name,
			tcfg:   tcfg,
			labels: prometheus.Labels(tcfg.Labels),
			cfg:    cfg,
		}
	}

	// Attempt initial connections concurrently in the background; the server
	// starts immediately. Failures are retried by the ticker below.
	for _, e := range targets {
		go e.connect()
	}

	// Background ticker retries disconnected targets every connect_timeout period.
	go func() {
		ticker := time.NewTicker(cfg.ConnectTimeout.Duration)
		defer ticker.Stop()
		for range ticker.C {
			for _, e := range targets {
				if !e.connected() {
					go e.connect()
				}
			}
		}
	}()

	mux := http.NewServeMux()
	mux.Handle("/metrics", metricsHandler(targets, cfg, logger))
	mux.Handle("/info", infoHandler(targets, cfg, logger))
	mux.HandleFunc("/targets", listTargets(targets))
	mux.HandleFunc("/-/healthy", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintln(w, "OK")
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `<html><head><title>Oracle Exporter</title></head><body>
<h1>Oracle Exporter</h1>
<p><a href="/targets">Configured targets</a></p>
<p>Metrics: <code>/metrics?target=&lt;name&gt;</code></p>
<p>Info (instance, PDBs, patches): <code>/info?target=&lt;name&gt;</code></p>
</body></html>`)
	})

	slog.Info("starting oracle exporter", "addr", *listenAddr)
	if err := http.ListenAndServe(*listenAddr, mux); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}

type targetEntry struct {
	name   string
	tcfg   config.TargetConfig
	labels prometheus.Labels
	cfg    *config.Config

	mu         sync.RWMutex
	t          *target.Target
	collectors []collector.Collector
}

func (e *targetEntry) get() (*target.Target, []collector.Collector) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.t, e.collectors
}

func (e *targetEntry) connected() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.t != nil
}

func (e *targetEntry) connect() {
	// Fast path: already connected.
	if e.connected() {
		return
	}
	// Network I/O happens outside any lock so readers are never blocked.
	if e.tcfg.UseWallet() {
		slog.Info("using wallet auth for target", "target", e.name, "TNS_ADMIN", os.Getenv("TNS_ADMIN"))
	}
	t, err := target.Open(e.name, e.tcfg, e.cfg.ConnectTimeout.Duration)
	if err != nil {
		slog.Warn("failed to connect to target", "target", e.name, "err", err)
		return
	}
	collectors, err := collector.Build(e.tcfg.Collectors.Apply(e.cfg.Collectors), t.Meta)
	if err != nil {
		slog.Error("failed to build collectors for target", "target", e.name, "err", err)
		t.DB.Close()
		return
	}
	// Write lock only to store the result.
	e.mu.Lock()
	if e.t == nil {
		e.t = t
		e.collectors = collectors
		slog.Info("target ready",
			"target", e.name,
			"db", t.Meta.DBName,
			"is_cdb", t.Meta.IsCDB,
			"is_asm", t.Meta.IsASM,
			"collectors", len(collectors),
		)
	} else {
		t.DB.Close() // lost the race with another goroutine
	}
	e.mu.Unlock()
}

func metricsHandler(targets map[string]*targetEntry, cfg *config.Config, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("target")
		if name == "" {
			http.Error(w, "missing ?target= parameter", http.StatusBadRequest)
			return
		}
		entry, ok := targets[name]
		if !ok {
			http.Error(w, fmt.Sprintf("unknown target %q", name), http.StatusNotFound)
			return
		}
		t, collectors := entry.get()
		if t == nil {
			http.Error(w, fmt.Sprintf("target %q is not connected", name), http.StatusServiceUnavailable)
			return
		}

		reg := prometheus.NewRegistry()
		registerer := prometheus.Registerer(reg)
		if len(entry.labels) > 0 {
			registerer = prometheus.WrapRegistererWith(entry.labels, reg)
		}
		registerer.MustRegister(
			collector.NewOracleCollector(t.DB, collectors, cfg.ScrapeTimeout.Duration, logger),
		)
		promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg}).ServeHTTP(w, r)
	}
}

func infoHandler(targets map[string]*targetEntry, cfg *config.Config, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("target")
		if name == "" {
			http.Error(w, "missing ?target= parameter", http.StatusBadRequest)
			return
		}
		entry, ok := targets[name]
		if !ok {
			http.Error(w, fmt.Sprintf("unknown target %q", name), http.StatusNotFound)
			return
		}
		t, _ := entry.get()
		if t == nil {
			http.Error(w, fmt.Sprintf("target %q is not connected", name), http.StatusServiceUnavailable)
			return
		}

		reg := prometheus.NewRegistry()
		registerer := prometheus.Registerer(reg)
		if len(entry.labels) > 0 {
			registerer = prometheus.WrapRegistererWith(entry.labels, reg)
		}
		registerer.MustRegister(
			collector.NewOracleCollector(t.DB, []collector.Collector{
				collector.NewInfoCollector(t.Meta.IsCDB),
			}, cfg.ScrapeTimeout.Duration, logger),
		)
		promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg}).ServeHTTP(w, r)
	}
}

func listTargets(targets map[string]*targetEntry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "target\tdb\tis_cdb\tis_asm\tconnected")
		for name, e := range targets {
			t, _ := e.get()
			if t == nil {
				_, _ = fmt.Fprintf(w, "%s\t-\t-\t-\tfalse\n", name)
			} else {
				_, _ = fmt.Fprintf(w, "%s\t%s\t%v\t%v\ttrue\n",
					name, t.Meta.DBName, t.Meta.IsCDB, t.Meta.IsASM)
			}
		}
	}
}
