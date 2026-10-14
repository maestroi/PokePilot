package main

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"

	appmetrics "github.com/maestroi/pokepilot/metrics"
)

// version is this build's git SHA, stamped by the Dockerfile via
// -ldflags "-X main.version=...". Local builds stay "dev".
var version = "dev"

type telegramCollector struct {
	src     *metrics
	pending func() float64

	updates       *prometheus.Desc
	commands      *prometheus.Desc
	unauthorized  *prometheus.Desc
	telegramErrs  *prometheus.Desc
	operatorErrs  *prometheus.Desc
	notifications *prometheus.Desc
	actions       *prometheus.Desc
	actionErrs    *prometheus.Desc
	pendingDesc   *prometheus.Desc
	wallHealthy   *prometheus.Desc
	lastPoll      *prometheus.Desc
}

func newTelegramCollector(b *bot) telegramCollector {
	desc := func(name, help string) *prometheus.Desc {
		return prometheus.NewDesc(name, help, nil, nil)
	}
	return telegramCollector{
		src: &b.m,
		pending: func() float64 {
			b.mu.Lock()
			defer b.mu.Unlock()
			return float64(len(b.confirmations))
		},
		updates:       desc("poketelegram_updates_total", "Telegram updates received."),
		commands:      desc("poketelegram_commands_total", "Commands handled."),
		unauthorized:  desc("poketelegram_unauthorized_total", "Callers rejected by the allowlist."),
		telegramErrs:  desc("poketelegram_telegram_errors_total", "Telegram API failures."),
		operatorErrs:  desc("poketelegram_operator_errors_total", "Operator API failures."),
		notifications: desc("poketelegram_notifications_total", "Notifications delivered."),
		actions:       desc("poketelegram_actions_total", "Confirmed control actions executed."),
		actionErrs:    desc("poketelegram_action_errors_total", "Control actions that failed."),
		pendingDesc:   desc("poketelegram_pending_confirmations", "Destructive actions awaiting confirmation."),
		wallHealthy:   desc("poketelegram_wall_healthy", "1 while the operator API is reachable."),
		lastPoll:      desc("poketelegram_last_operator_poll_timestamp_seconds", "Unix time of the last successful operator poll."),
	}
}

func (c telegramCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range []*prometheus.Desc{
		c.updates, c.commands, c.unauthorized, c.telegramErrs, c.operatorErrs,
		c.notifications, c.actions, c.actionErrs, c.pendingDesc, c.wallHealthy, c.lastPoll,
	} {
		ch <- d
	}
}

func (c telegramCollector) Collect(ch chan<- prometheus.Metric) {
	counter := func(d *prometheus.Desc, v uint64) {
		ch <- prometheus.MustNewConstMetric(d, prometheus.CounterValue, float64(v))
	}
	counter(c.updates, c.src.updates.Load())
	counter(c.commands, c.src.commands.Load())
	counter(c.unauthorized, c.src.unauthorized.Load())
	counter(c.telegramErrs, c.src.telegramErrs.Load())
	counter(c.operatorErrs, c.src.operatorErrs.Load())
	counter(c.notifications, c.src.notifications.Load())
	counter(c.actions, c.src.actions.Load())
	counter(c.actionErrs, c.src.actionErrs.Load())
	ch <- prometheus.MustNewConstMetric(c.pendingDesc, prometheus.GaugeValue, c.pending())
	ch <- prometheus.MustNewConstMetric(c.wallHealthy, prometheus.GaugeValue, float64(c.src.wallHealthy.Load()))
	ch <- prometheus.MustNewConstMetric(c.lastPoll, prometheus.GaugeValue, float64(c.src.lastPollUnix.Load()))
}

func (b *bot) prometheusHandler() http.Handler {
	reg := appmetrics.Must(appmetrics.Options{Service: appmetrics.ServiceTelegram, Revision: version})
	if err := reg.Register(newTelegramCollector(b)); err != nil {
		panic(err)
	}
	return reg.Handler()
}
