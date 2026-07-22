package remotecontrol

import (
	"fmt"
	"os"
	"strconv"
	"time"

	protocol "github.com/colinagent/openbrain/opagent-protocol/go-sdk/remotecontrol"
)

const (
	enabledEnvironmentVariable    = "OPENBRAIN_REMOTE_CONTROL_ENABLED"
	killSwitchEnvironmentVariable = "OPENBRAIN_REMOTE_CONTROL_KILL_SWITCH"
	apiURLEnvironmentVariable     = "OPENBRAIN_REMOTE_CONTROL_API_URL"
	defaultAPIURL                 = "https://api.op-agent.com/v1/remote-control"
)

type Config struct {
	Enabled              bool
	KillSwitch           bool
	APIURL               string
	ReplayWindow         time.Duration
	MaxReplayEntries     int
	MaxReplayResultBytes int
}

func DefaultConfig() Config {
	return Config{
		Enabled:              false,
		KillSwitch:           true,
		APIURL:               defaultAPIURL,
		ReplayWindow:         time.Duration(protocol.RequestReplayWindowSeconds) * time.Second,
		MaxReplayEntries:     256,
		MaxReplayResultBytes: 64 * 1024,
	}
}

func ConfigFromEnvironment() (Config, error) {
	config := DefaultConfig()
	var err error

	if raw, ok := os.LookupEnv(enabledEnvironmentVariable); ok {
		config.Enabled, err = strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("parse %s: %w", enabledEnvironmentVariable, err)
		}
	}
	if raw, ok := os.LookupEnv(killSwitchEnvironmentVariable); ok {
		config.KillSwitch, err = strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("parse %s: %w", killSwitchEnvironmentVariable, err)
		}
	}
	if raw, ok := os.LookupEnv(apiURLEnvironmentVariable); ok && raw != "" {
		config.APIURL = raw
	}

	return config, nil
}

func (c Config) AllowsRemoteControl() bool {
	return c.Enabled && !c.KillSwitch
}
