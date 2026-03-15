package notifier

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/sbkg0002/beer-alerter/internal/config"
	"github.com/sbkg0002/beer-alerter/internal/scraper"
)

type Notifier struct {
	cfg    config.NtfyConfig
	client *http.Client
}

func New(cfg config.NtfyConfig) *Notifier {
	return &Notifier{
		cfg:    cfg,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (n *Notifier) Notify(ctx context.Context, beers []scraper.Beer) error {
	var lines []string
	for _, b := range beers {
		line := fmt.Sprintf("%s - %s", b.BreweryName, b.BeerName)
		if b.BeerStyle != "" || b.BeerABV != "" {
			line += fmt.Sprintf(" (%s", b.BeerStyle)
			if b.BeerABV != "" {
				if b.BeerStyle != "" {
					line += ", "
				}
				line += b.BeerABV + "%"
			}
			line += ")"
		}
		lines = append(lines, line)
	}

	body := strings.Join(lines, "\n")
	title := fmt.Sprintf("Beer alert: %d match(es) on draft", len(beers))
	url := n.cfg.BaseURL + "/" + n.cfg.Topic

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Title", title)
	req.Header.Set("Priority", n.cfg.Priority)
	req.Header.Set("Content-Type", "text/plain")
	if len(n.cfg.Tags) > 0 {
		req.Header.Set("Tags", strings.Join(n.cfg.Tags, ","))
	}

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("send notification: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ntfy returned status %d", resp.StatusCode)
	}

	return nil
}
