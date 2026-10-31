package notify

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Send posts a message to ntfy. Empty server/topic/token = disabled (nil).
func Send(ctx context.Context, server, topic, token, title, priority, tags, message string) error {
	server = strings.TrimRight(strings.TrimSpace(server), "/")
	topic = strings.TrimSpace(topic)
	if server == "" || topic == "" {
		return nil // disabled
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server+"/"+topic, strings.NewReader(message))
	if err != nil {
		return err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("User-Agent", "dnsupdater/1.0")
	if title != "" {
		req.Header.Set("Title", title)
	}
	if priority != "" {
		req.Header.Set("Priority", priority)
	}
	if tags != "" {
		req.Header.Set("Tags", tags)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("ntfy: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ntfy HTTP %d", resp.StatusCode)
	}
	return nil
}
