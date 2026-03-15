package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/sbkg0002/beer-alerter/internal/config"
)

type Beer struct {
	BreweryName string
	BeerName    string
	BeerStyle   string
	BeerABV     string
}

type Scraper struct {
	cfg config.ScrapeConfig
}

func New(cfg config.ScrapeConfig) *Scraper {
	return &Scraper{cfg: cfg}
}

// DumpHTML navigates to the venue URL, waits for JS rendering, and returns the full page HTML.
// Used for debugging selector issues.
func (s *Scraper) DumpHTML(ctx context.Context) (string, error) {
	page, cleanup, err := s.newPage()
	if err != nil {
		return "", err
	}
	defer cleanup()

	if err := s.navigateAndWait(page); err != nil {
		return "", err
	}
	return page.HTML()
}

func (s *Scraper) newPage() (*rod.Page, func(), error) {
	l := launcher.New().
		Bin("/usr/bin/chromium").
		Headless(true).
		NoSandbox(true).
		Set("disable-gpu", "").
		Set("disable-dev-shm-usage", "").
		Set("no-zygote", "")

	u, err := l.Launch()
	if err != nil {
		slog.Warn("could not launch /usr/bin/chromium, falling back to auto-detect", "error", err)
		l2 := launcher.New().
			Headless(true).
			NoSandbox(true).
			Set("disable-gpu", "").
			Set("disable-dev-shm-usage", "").
			Set("no-zygote", "")
		u, err = l2.Launch()
		if err != nil {
			return nil, nil, fmt.Errorf("launch browser: %w", err)
		}
	}

	browser := rod.New().ControlURL(u)
	if err := browser.Connect(); err != nil {
		return nil, nil, fmt.Errorf("connect to browser: %w", err)
	}

	timeout := time.Duration(s.cfg.PageTimeoutSeconds) * time.Second
	page, err := browser.Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		browser.MustClose()
		return nil, nil, fmt.Errorf("create page: %w", err)
	}
	page = page.Timeout(timeout)

	if err := page.SetUserAgent(&proto.NetworkSetUserAgentOverride{
		UserAgent: "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	}); err != nil {
		browser.MustClose()
		return nil, nil, fmt.Errorf("set user agent: %w", err)
	}

	return page, func() { browser.MustClose() }, nil
}

func (s *Scraper) navigateAndWait(page *rod.Page) error {
	if err := page.Navigate(s.cfg.URL); err != nil {
		return fmt.Errorf("navigate to %s: %w", s.cfg.URL, err)
	}

	// Wait for initial page load
	page.MustWaitLoad()

	// Wait for network requests (XHR) to settle — this is what actually
	// triggers the Handlebars menu rendering on Untappd
	waitIdle := page.WaitRequestIdle(500*time.Millisecond, nil, nil, nil)
	waitIdle()

	return nil
}

func (s *Scraper) Scrape(ctx context.Context) ([]Beer, error) {
	page, cleanup, err := s.newPage()
	if err != nil {
		return nil, err
	}
	defer cleanup()

	if err := s.navigateAndWait(page); err != nil {
		return nil, err
	}

	// Dump page HTML at debug level so selectors can be inspected
	if slog.Default().Enabled(ctx, slog.LevelDebug) {
		if html, err := page.HTML(); err == nil {
			slog.Debug("page HTML after render", "html", html)
		}
	}

	// Each section is a div.menu-section; find the one whose h4 header matches
	// the configured draft_section string (e.g. "on draft").
	sections, err := page.Elements("div.menu-section")
	if err != nil {
		return nil, fmt.Errorf("find menu sections: %w", err)
	}

	slog.Debug("menu sections found", "count", len(sections))

	var draftSection *rod.Element
	for _, section := range sections {
		headerEl, err := section.Element("div.menu-section-header h4")
		if err != nil {
			continue
		}
		headerText, err := headerEl.Text()
		if err != nil {
			continue
		}
		if strings.Contains(strings.ToLower(headerText), strings.ToLower(s.cfg.DraftSection)) {
			draftSection = section
			slog.Debug("found draft section", "header", strings.TrimSpace(headerText))
			break
		}
	}

	if draftSection == nil {
		return nil, fmt.Errorf("could not find section matching %q — check page HTML in verbose mode", s.cfg.DraftSection)
	}

	items, err := draftSection.Elements("li.menu-item")
	if err != nil {
		return nil, fmt.Errorf("find beer items in draft section: %w", err)
	}

	slog.Debug("found items in draft section", "count", len(items))

	var beers []Beer
	for _, item := range items {
		beer, err := extractBeer(item)
		if err != nil {
			slog.Warn("failed to extract beer from item, skipping", "error", err)
			continue
		}
		if beer.BeerName != "" {
			beers = append(beers, beer)
		}
	}

	return beers, nil
}

type beerData struct {
	Name    string `json:"name"`
	Brewery string `json:"brewery"`
	Style   string `json:"style"`
	ABV     string `json:"abv"`
}

func extractBeer(item *rod.Element) (Beer, error) {
	// Actual rendered structure (confirmed from page.html):
	//   li.menu-item
	//     div.beer-details
	//       h5
	//         a  → "1. Beer Name"  (strip leading "N. " prefix)
	//         em → "Style"
	//       h6
	//         span → "6.5% ABV • 22 IBU • [brewery link] •"
	//           a  → "Brewery Name"
	result, err := item.Eval(`() => {
		const d = this.querySelector('.beer-details');
		if (!d) return JSON.stringify({});

		const nameEl   = d.querySelector('h5 a');
		const styleEl  = d.querySelector('h5 em');
		const breweryEl = d.querySelector('h6 span a');
		const abvSpan  = d.querySelector('h6 span');

		// Beer name has a leading "N. " position number — strip it
		const rawName = nameEl ? nameEl.innerText.trim() : '';
		const name = rawName.replace(/^\d+\.\s*/, '');

		// ABV is in the format "6.5% ABV • N/A IBU • ..."
		const abvText = abvSpan ? abvSpan.innerText.trim() : '';
		const abvMatch = abvText.match(/([\d.]+)%\s*ABV/);
		const abv = abvMatch ? abvMatch[1] : '';

		return JSON.stringify({
			name:    name,
			brewery: breweryEl ? breweryEl.innerText.trim() : '',
			style:   styleEl  ? styleEl.innerText.trim()   : '',
			abv:     abv,
		});
	}`)
	if err != nil {
		return Beer{}, fmt.Errorf("eval beer data: %w", err)
	}

	var data beerData
	if err := json.Unmarshal([]byte(result.Value.Str()), &data); err != nil {
		return Beer{}, fmt.Errorf("unmarshal beer data: %w", err)
	}

	return Beer{
		BreweryName: data.Brewery,
		BeerName:    data.Name,
		BeerStyle:   data.Style,
		BeerABV:     data.ABV,
	}, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
