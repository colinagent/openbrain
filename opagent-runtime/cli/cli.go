package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/colinagent/openbrain/opagent-runtime/internal/config"
	"github.com/colinagent/openbrain/opagent-runtime/internal/logger"
	"github.com/colinagent/openbrain/opagent-runtime/internal/memory/cache"
	"github.com/colinagent/openbrain/opagent-runtime/internal/run"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "opagent-runtime",
	Short: "Start the OpAgent runtime",
	Long:  `OpAgent Runtime`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		preRun()
	},
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Starting opagent-runtime...")
		run.Start()
	},
}

// Execute runs the root command and returns any error (caller may os.Exit(1)).
func Execute() error {
	return rootCmd.Execute()
}

func preRun() {
	configDir := filepath.Join(config.CmdBaseDir, "configs")
	if err := config.Parse(configDir); err != nil {
		fmt.Fprintf(os.Stderr, "Error: parse config failed: %v\n", err)
		os.Exit(1)
	}
	cfg := config.GetConfig()
	if cfg == nil {
		fmt.Fprintln(os.Stderr, "Error: system config not initialized")
		os.Exit(1)
	}
	logDir := filepath.Join(config.CmdBaseDir, "logs", "opagent-runtime")
	logger.InitLogger(cfg.System.Debug, filepath.Join(logDir, "opagent-runtime.log"))

	cache.NewCache(nil)
	cache.Set("config", cache.PrefixDefault, cfg, cache.NoExpiration)
	secrets := config.GetSecrets()
	cache.Set("secrets", cache.PrefixDefault, secrets, cache.NoExpiration)

}

func init() {
	rootCmd.PersistentFlags().BoolVar(&config.CmdDebug, "debug", config.DefaultDebug, fmt.Sprintf("debug mode, default is %t", config.DefaultDebug))

	rootCmd.PersistentFlags().StringVarP(&config.CmdBaseDir, "base-dir", "b", config.DefaultBaseDir, fmt.Sprintf("base directory path, default is %s", config.DefaultBaseDir))

	rootCmd.PersistentFlags().StringVar(&config.CmdEnv, "env", string(config.DefaultEnv), fmt.Sprintf("runtime environment (local|cloud), default is %s", string(config.DefaultEnv)))

	rootCmd.PersistentFlags().StringVar(&config.CmdCloudOSBaseURL, "cloudos-base-url", config.DefaultCloudOSBaseURL, "cloudos base URL for cloud env, e.g. http://127.0.0.1:8080")
}
