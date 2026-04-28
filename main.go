package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"

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

	// Open all targets at startup. A failed target is logged but does not
	// prevent the exporter from serving other targets.
	targets := make(map[string]*targetEntry, len(cfg.Targets))
	for name, tcfg := range cfg.Targets {
		if tcfg.UseWallet() {
			slog.Info("using wallet auth for target", "target", name, "TNS_ADMIN", os.Getenv("TNS_ADMIN"))
		}
		t, err := target.Open(name, tcfg, cfg.ScrapeTimeout.Duration)
		if err != nil {
			slog.Warn("failed to connect to target, skipping", "target", name, "err", err)
			continue
		}
		collectors, err := collector.Build(tcfg.Collectors.Apply(cfg.Collectors), t.Meta)
		if err != nil {
			slog.Error("failed to build collectors for target", "target", name, "err", err)
			t.DB.Close()
			continue
		}
		targets[name] = &targetEntry{target: t, collectors: collectors}
		slog.Info("target ready",
			"target", name,
			"db", t.Meta.DBName,
			"is_cdb", t.Meta.IsCDB,
			"is_asm", t.Meta.IsASM,
			"collectors", len(collectors),
		)
	}

	if len(targets) == 0 {
		slog.Error("no targets available, exiting")
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.Handle("/metrics", metricsHandler(targets, cfg, logger))
	mux.HandleFunc("/targets", listTargets(targets))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `<html><head><title>Oracle Exporter</title></head><body>
<h1>Oracle Exporter</h1>
<p><a href="/targets">Configured targets</a></p>
<p>Metrics: <code>/metrics?target=&lt;name&gt;</code></p>
</body></html>`)
	})

	slog.Info("starting oracle exporter", "addr", *listenAddr)
	if err := http.ListenAndServe(*listenAddr, mux); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}

type targetEntry struct {
	target     *target.Target
	collectors []collector.Collector
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

		reg := prometheus.NewRegistry()
		reg.MustRegister(
			collector.NewOracleCollector(entry.target.DB, entry.collectors, cfg.ScrapeTimeout.Duration, logger),
		)
		promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg}).ServeHTTP(w, r)
	}
}

func listTargets(targets map[string]*targetEntry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "target\tdb\tis_cdb\tis_asm")
		for name, e := range targets {
			_, _ = fmt.Fprintf(w, "%s\t%s\t%v\t%v\n",
				name, e.target.Meta.DBName, e.target.Meta.IsCDB, e.target.Meta.IsASM)
		}
	}
}
