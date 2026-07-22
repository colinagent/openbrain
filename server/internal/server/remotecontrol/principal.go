package remotecontrol

import (
	"fmt"

	protocol "github.com/colinagent/openbrain/opagent-protocol/go-sdk/remotecontrol"
)

// Principal is the authenticated remote identity presented to host handlers.
// It deliberately carries no account token or relay credential.
type Principal struct {
	UID           string
	EnvironmentID string
	ClientID      string
	capabilities  map[protocol.Capability]struct{}
}

func NewPrincipal(uid, environmentID, clientID string, capabilities ...protocol.Capability) (Principal, error) {
	if uid == "" || environmentID == "" || clientID == "" {
		return Principal{}, fmt.Errorf("remote principal requires uid, environmentID, and clientID")
	}

	granted := make(map[protocol.Capability]struct{}, len(capabilities))
	for _, capability := range capabilities {
		if capability == "" {
			return Principal{}, fmt.Errorf("remote principal capability cannot be empty")
		}
		granted[capability] = struct{}{}
	}

	return Principal{
		UID:           uid,
		EnvironmentID: environmentID,
		ClientID:      clientID,
		capabilities:  granted,
	}, nil
}

func (p Principal) HasCapability(capability protocol.Capability) bool {
	_, ok := p.capabilities[capability]
	return ok
}
