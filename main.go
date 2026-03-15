package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/robfig/cron/v3"
	"github.com/sbkg0002/beer-alerter/internal/config"
	"github.com/sbkg0002/beer-alerter/internal/notifier"
	"github.com/sbkg0002/beer-alerter/internal/scraper"
)

func main() {
	once     := flag.Bool("once", false, "run once and exit instead of starting the scheduler")
	verbose  := flag.Bool("verbose", false, "enable debug logging")
	dumpHTML := flag.String("dump-html", "", "write rendered page HTML to this file path (for debugging)")
	flag.Parse()

	if *verbose {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})))
	}

	cfgPath := os.Getenv("CONFIG_PATH")
	if cfgPath == "" {
		cfgPath = "/app/config.yaml"
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		slog.Error("failed to load config", "path", cfgPath, "error", err)
		os.Exit(1)
	}

	scrpr := scraper.New(cfg.Scrape)
	ntfr := notifier.New(cfg.Ntfy)

	if *dumpHTML != "" {
		html, err := scrpr.DumpHTML(context.Background())
		if err != nil {
			slog.Error("dump HTML failed", "error", err)
			os.Exit(1)
		}
		if err := os.WriteFile(*dumpHTML, []byte(html), 0644); err != nil {
			slog.Error("write HTML file failed", "error", err)
			os.Exit(1)
		}
		slog.Info("HTML written", "path", *dumpHTML)
		return
	}

	if *once {
		runJob(scrpr, ntfr, cfg.Brewers)
		return
	}

	// Run once immediately so we get feedback right away
	runJob(scrpr, ntfr, cfg.Brewers)

	c := cron.New()
	if _, err := c.AddFunc(cfg.Schedule.Cron, func() {
		runJob(scrpr, ntfr, cfg.Brewers)
	}); err != nil {
		slog.Error("invalid cron expression", "expr", cfg.Schedule.Cron, "error", err)
		os.Exit(1)
	}

	c.Start()
	slog.Info("scheduler started", "cron", cfg.Schedule.Cron)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down")
	ctx := c.Stop()
	<-ctx.Done()
}

func runJob(s *scraper.Scraper, n *notifier.Notifier, brewers []string) {
	slog.Info("scrape run starting")

	beers, err := s.Scrape(context.Background())
	if err != nil {
		slog.Error("scrape failed", "error", err)
		return
	}

	slog.Info("scraped beers from draft section", "count", len(beers))
	for _, b := range beers {
		slog.Info("on draft", "brewery", b.BreweryName, "beer", b.BeerName, "style", b.BeerStyle, "abv", b.BeerABV)
	}

	var matches []scraper.Beer
	for _, b := range beers {
		if matchesBrewers(b, brewers) {
			matches = append(matches, b)
			slog.Info("match found", "brewery", b.BreweryName, "beer", b.BeerName)
		}
	}

	slog.Info("run complete", "matches", len(matches))

	if len(matches) == 0 {
		return
	}

	if err := n.Notify(context.Background(), matches); err != nil {
		slog.Error("notification failed", "error", err)
	} else {
		slog.Info("notification sent", "count", len(matches))
	}
}

func matchesBrewers(beer scraper.Beer, brewers []string) bool {
	name := strings.ToLower(beer.BreweryName)
	for _, b := range brewers {
		if strings.Contains(name, strings.ToLower(b)) {
			return true
		}
	}
	return false
}
