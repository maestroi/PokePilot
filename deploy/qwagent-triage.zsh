# PokePilot qwagent farm-triage loop. Timer is opt-in.
alias qwtriage-on='systemctl --user enable --now qwagent-triage.timer'
alias qwtriage-off='systemctl --user disable --now qwagent-triage.timer'
alias qwtriage-once='systemctl --user start qwagent-triage.service'
alias qwtriage-status='systemctl --user status qwagent-triage.timer qwagent-triage.service'
alias qwtriage-logs='journalctl --user -u qwagent-triage.service -u qwagent-triage.timer -f'
