# Terminal demo for the README. Render with: vhs docs/demo.tape
# Needs the Compose stack running (docker compose up -d).
Output docs/img/demo.gif

Set Shell "bash"
Set FontSize 15
Set Width 1200
Set Height 720
Set Padding 20
Set Theme "Catppuccin Mocha"
Set TypingSpeed 35ms

Hide
Type "export PS1='$ ' && clear"
Enter
Show

Type "# NetPulse: concurrent ICMP / TCP / DNS prober in Go"
Enter
Sleep 800ms

Type "go test ./... -race -count=1"
Enter
Sleep 9s

Type "curl -s localhost:9101/metrics | grep -E '^netpulse_(probe_up|packets_)'"
Enter
Sleep 4s

Type "# BGP lab: silent link failure, outage until failover"
Enter
Type "sed -n '/## Summary/,$p' results/20260928-133143-bgp.md"
Enter
Sleep 6s
