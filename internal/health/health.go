package health

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"time"
)

// Check polls the given URL until it returns a 2xx status code.
// It retries up to maxAttempts times with the given interval between attempts.
func Check(ctx context.Context, url string, maxAttempts int, interval time.Duration) error {
	// Skip TLS verification for internal/self-signed certs (same as curl -k).
	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("health check cancelled: %w", err)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return fmt.Errorf("create health check request: %w", err)
		}

		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				fmt.Printf("    Health check passed (attempt %d/%d)\n", attempt, maxAttempts)
				return nil
			}
		}

		if attempt < maxAttempts {
			time.Sleep(interval)
		}
	}

	return fmt.Errorf("health check failed after %d attempts against %s", maxAttempts, url)
}
