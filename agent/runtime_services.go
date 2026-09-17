package agent

import (
	"os"
	"strings"
)

const virtualTraderURLEnv = "POKEPILOT_VIRTUAL_TRADER_URL"

// runtimeServicesFromEnvironment projects configured infrastructure into the
// planner observation. The endpoint itself stays private to the runner; only
// the semantic capability crosses the planner contract.
func runtimeServicesFromEnvironment() *RuntimeServices {
	if strings.TrimSpace(os.Getenv(virtualTraderURLEnv)) == "" {
		return nil
	}
	return &RuntimeServices{VirtualTrader: true}
}
