# beer-alerter

Monitors a beer venue's draft list on Untappd and sends a push notification via [ntfy.sh](https://ntfy.sh) when beers from configured brewers appear on tap.

## Requirements

- Docker

## Setup

```bash
cp config.example.yaml config.yaml
# Edit config.yaml: set your brewers list and ntfy topic
```

## Configuration

```yaml
scrape:
  url: "https://untappd.com/v/brouwcafe-de-molen/75672"
  draft_section: "on draft" # case-insensitive substring match on section heading
  page_timeout_seconds: 30

schedule:
  cron: "*/30 * * * *" # standard 5-field cron; every 30 minutes

brewers:
  - "De Molen"
  - "Verdant"

ntfy:
  topic: "your-topic"
  base_url: "https://ntfy.sh" # override for self-hosted ntfy
  priority: "default" # min | low | default | high | max
  tags:
    - "beer"
    - "tada"
```

## Running with Docker Compose

```bash
docker compose up -d        # start in background
docker compose logs -f      # follow logs
docker compose down         # stop
```

## Running with Docker

```bash
docker run -d \
  --shm-size=256m \
  -v $(pwd)/config.yaml:/app/config.yaml:ro \
  sbkg0002/beer-alerter
```

## Flags

| Flag                 | Description                                                  |
| -------------------- | ------------------------------------------------------------ |
| `--once`             | Run once and exit instead of starting the scheduler          |
| `--verbose`          | Enable debug logging                                         |
| `--dump-html <path>` | Write rendered page HTML to a file (for debugging selectors) |

## Debugging

Dump the rendered page HTML to inspect selectors:

```bash
docker run --rm --shm-size=256m \
  -v $(pwd)/config.yaml:/app/config.yaml:ro \
  -v $(pwd):/out \
  sbkg0002/beer-alerter --dump-html /out/page.html
```
