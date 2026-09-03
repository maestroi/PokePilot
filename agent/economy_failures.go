package agent

import (
	"fmt"
	"strings"
)

// plannerEconomyContext applies run-history facts to the pure resource policy.
// Offer still leaves a failed purchase legal: conditions can change. But after
// the same item has failed twice the economy advice stops recommending it, so
// the planner sees both the permanent failure tally and ShouldBuy=false instead
// of receiving contradictory guidance that can become a shopping loop.
func plannerEconomyContext(o Observation) *EconomyDecisionContext {
	ctx := EconomyContext(o)
	if ctx == nil {
		return nil
	}
	for i := range ctx.Purchases {
		failures := purchaseFailureCount(o, ctx.Purchases[i].Item)
		if failures < 2 || !ctx.Purchases[i].ShouldBuy {
			continue
		}
		ctx.Purchases[i].ShouldBuy = false
		ctx.Purchases[i].SuggestedQty = 0
		ctx.Purchases[i].SuggestedCost = 0
		ctx.Purchases[i].Reason = fmt.Sprintf("this purchase has already failed %dx; do not repeat it until the situation changes", failures)
	}
	return ctx
}

func purchaseFailureCount(o Observation, item string) int {
	needle := strings.ToLower(item)
	count := 0
	for _, failure := range o.Failures {
		objective := strings.ToLower(failure.Objective)
		if strings.HasPrefix(objective, "buy ") && strings.Contains(objective, needle) && failure.Times > count {
			count = failure.Times
		}
	}
	return count
}
