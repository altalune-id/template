package cli

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"altalune.id/template/internal/cli/render"
)

func newHealthzCmd() *cobra.Command {
	var (
		url     string
		timeout time.Duration
	)

	cmd := &cobra.Command{
		Use:     "healthz",
		Short:   "Probe /healthz and exit 0 iff the server is healthy",
		GroupID: "meta",
		RunE: func(cmd *cobra.Command, _ []string) error {
			target := url
			if target == "" {
				target = defaultHealthzURL(cmd.Context())
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, http.NoBody)
			if err != nil {
				return fmt.Errorf("healthz: build request: %w", err)
			}
			start := time.Now()
			resp, doErr := http.DefaultClient.Do(req)
			elapsed := time.Since(start).Truncate(time.Millisecond)
			status := 0
			probeErr := ""
			if doErr != nil {
				probeErr = doErr.Error()
			} else {
				status = resp.StatusCode
				_ = resp.Body.Close()
			}
			ok := doErr == nil && status >= 200 && status < 300

			switch render.Detect(cmd) {
			case render.FormatJSON, render.FormatNDJSON:
				_ = render.JSON(cmd.OutOrStdout(), map[string]any{
					"url":    target,
					"status": status,
					"ok":     ok,
					"took":   elapsed.String(),
					"error":  probeErr,
				})
			default:
				switch {
				case ok:
					cmd.Printf("ok   %s  (%d, %s)\n", target, status, elapsed)
				case probeErr != "":
					cmd.Printf("fail %s  (%s, %s)\n", target, probeErr, elapsed)
				default:
					cmd.Printf("fail %s  (%d, %s)\n", target, status, elapsed)
				}
			}

			if !ok {
				return errHealthzUnhealthy
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&url, "url", "",
		"health URL to probe (default: http://127.0.0.1:<http.addr port>/healthz)")
	cmd.Flags().DurationVar(&timeout, "timeout", 3*time.Second, "request timeout")
	return cmd
}

var errHealthzUnhealthy = errors.New("healthz: unhealthy")

func defaultHealthzURL(ctx context.Context) string {
	addr := ":5150"
	if cfg := configFromCtx(ctx); cfg != nil && cfg.HTTP.Addr != "" {
		addr = cfg.HTTP.Addr
	}
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		port = strings.TrimPrefix(addr, ":")
	}
	return "http://127.0.0.1:" + port + "/healthz"
}
