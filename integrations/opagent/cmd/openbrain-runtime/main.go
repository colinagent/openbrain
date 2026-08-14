package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	agentruntime "github.com/colinagent/opagent/opagent-runtime/runtime"
	product "github.com/colinagent/openbrain/integrations/opagent"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "openbrain runtime: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("opagent-runtime", flag.ContinueOnError)
	baseDir := flags.String("base-dir", "", "OpenBrain product base directory")
	env := flags.String("env", "local", "Runtime environment")
	debug := flags.Bool("debug", false, "enable debug logging")
	if err := flags.Parse(args); err != nil {
		return err
	}
	options, err := product.ProductRuntimeOptions(*baseDir)
	if err != nil {
		return err
	}
	options.Env = *env
	options.Debug = *debug
	runtime, err := agentruntime.New(options)
	if err != nil {
		return err
	}
	runCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	return runtime.Run(runCtx)
}
