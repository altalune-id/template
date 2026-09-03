package cli

import (
	"github.com/spf13/cobra"
)

func newServeCmd(bootServer ServerBootFn) *cobra.Command {
	return &cobra.Command{
		Use:     "serve",
		Short:   "Run the altempl HTTP server (web UI + Connect API + workers)",
		GroupID: "runtime",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := withCfg(cmd)
			if err != nil {
				return err
			}
			s, err := bootServer(cmd.Context(), cfg)
			if err != nil {
				return err
			}
			defer func() { _ = s.Close() }()

			cmd.Printf("altempl: listening on %s (basePath=%q, mode=%s)\n",
				s.Cfg.HTTP.Addr, s.Cfg.HTTP.BasePath, s.Cfg.Mode)
			return s.Run(cmd.Context())
		},
	}
}
