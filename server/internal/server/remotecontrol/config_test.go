package remotecontrol

import "testing"

func TestConfigFromEnvironmentDefaultsClosed(t *testing.T) {
	t.Setenv(enabledEnvironmentVariable, "false")
	t.Setenv(killSwitchEnvironmentVariable, "true")
	config, err := ConfigFromEnvironment()
	if err != nil {
		t.Fatalf("ConfigFromEnvironment: %v", err)
	}
	if config.AllowsRemoteControl() {
		t.Fatal("default configuration allows remote control")
	}
}

func TestConfigFromEnvironmentRequiresBothGates(t *testing.T) {
	t.Setenv(enabledEnvironmentVariable, "true")
	t.Setenv(killSwitchEnvironmentVariable, "false")
	config, err := ConfigFromEnvironment()
	if err != nil {
		t.Fatalf("ConfigFromEnvironment: %v", err)
	}
	if !config.AllowsRemoteControl() {
		t.Fatal("explicitly enabled configuration is closed")
	}
}

func TestConfigFromEnvironmentRejectsInvalidBoolean(t *testing.T) {
	t.Setenv(enabledEnvironmentVariable, "sometimes")
	if _, err := ConfigFromEnvironment(); err == nil {
		t.Fatal("invalid enabled flag was accepted")
	}
}
