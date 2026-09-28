#!/usr/bin/env bash
# Inject a tc netem impairment into the NetPulse container, watch Prometheus
# for alerts, and write a markdown report of time-to-detect and measured
# impact.
#
#   scripts/fault-inject.sh "delay 100ms 20ms loss 10%" 420
#
# Arg 1: netem parameters (applied to egress on eth0 of the netpulse container)
# Arg 2: how long to hold the fault, in seconds (default 420)
#
# Needs: docker compose stack running, jq, curl.
set -euo pipefail

NETEM=${1:?usage: $0 "<netem params>" [duration_seconds]}
DURATION=${2:-420}
PROM=${PROM:-http://localhost:9090}
POLL=5

cd "$(dirname "$0")/.."
CID=$(docker compose ps -q netpulse)
[[ -n "$CID" ]] || { echo "netpulse container not running" >&2; exit 1; }

tc_in_netpulse() {
  docker run --rm --net "container:$CID" --cap-add NET_ADMIN nicolaka/netshoot tc "$@"
}

# PromQL instant query -> "target (type) value" lines
q() {
  curl -sfG "$PROM/api/v1/query" --data-urlencode "query=$1" |
    jq -r '.data.result[] | "\(.metric.target) (\(.metric.type))\t\(.value[1])"' | sort
}

p95() {
  q "histogram_quantile(0.95, sum by (le, target, type) (rate(netpulse_probe_latency_seconds_bucket[$1])))"
}
mean() {
  q "rate(netpulse_probe_latency_seconds_sum[$1]) / rate(netpulse_probe_latency_seconds_count[$1])"
}
loss() {
  q "1 - increase(netpulse_packets_received_total[$1]) / increase(netpulse_packets_sent_total[$1])"
}

cleanup() { tc_in_netpulse qdisc del dev eth0 root 2>/dev/null || true; }
trap cleanup EXIT

mkdir -p results
REPORT="results/$(date +%Y%m%d-%H%M%S)-fault.md"
WIN="${DURATION}s"

echo "Baseline (last ${WIN})..."
BASE_P95=$(p95 "$WIN"); BASE_MEAN=$(mean "$WIN"); BASE_LOSS=$(loss "$WIN")

cleanup
echo "Injecting: netem $NETEM"
tc_in_netpulse qdisc add dev eth0 root netem $NETEM
T0=$(date +%s)

# Log every (elapsed, alert, state) observation; awk picks the first of each.
# (Plain files rather than associative arrays: macOS ships bash 3.2.)
EVENTS=$(mktemp)
while (( $(date +%s) - T0 < DURATION )); do
  now=$(( $(date +%s) - T0 ))
  curl -sf "$PROM/api/v1/alerts" |
    jq -r --arg t "$now" '.data.alerts[] |
      "\($t)\t\(.labels.alertname) \(.labels.target // "-") (\(.labels.type // "-"))\t\(.state)"' |
    tee -a "$EVENTS" | awk -F'\t' '{print "  +" $1 "s  " $3 "\t" $2}'
  sleep "$POLL"
done

echo "Measuring impact over the fault window..."
FAULT_P95=$(p95 "$WIN"); FAULT_MEAN=$(mean "$WIN"); FAULT_LOSS=$(loss "$WIN")

{
  echo "# Fault injection: \`netem $NETEM\`"
  echo
  echo "- Date: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "- Fault held for ${DURATION}s; alerts polled every ${POLL}s (Prometheus scrape/eval 5s)"
  echo
  echo "## Time to detect"
  echo
  echo "| Alert | Pending after | Firing after |"
  echo "|---|---|---|"
  awk -F'\t' -v dur="$DURATION" '
    !($2 in seen) { seen[$2] = 1; order[++n] = $2; pend[$2] = $1 }
    $3 == "firing" && !($2 in fire) { fire[$2] = $1 }
    END {
      if (n == 0) print "| (none fired) | | |"
      for (i = 1; i <= n; i++) {
        k = order[i]
        printf "| %s | %ss | %s |\n", k, pend[k], (k in fire) ? fire[k] "s" : "not within " dur "s"
      }
    }' "$EVENTS"
  echo
  echo "## Measured impact (window = ${WIN})"
  echo
  echo "| Target | Mean before | Mean during | p95 before | p95 during | Loss before | Loss during |"
  echo "|---|---|---|---|---|---|---|"
  join -t $'\t' <(echo "$BASE_MEAN") <(echo "$FAULT_MEAN") |
    join -t $'\t' - <(echo "$BASE_P95") | join -t $'\t' - <(echo "$FAULT_P95") |
    join -t $'\t' - <(echo "$BASE_LOSS") | join -t $'\t' - <(echo "$FAULT_LOSS") |
    awk -F'\t' '{printf "| %s | %.1f ms | %.1f ms | %.1f ms | %.1f ms | %.1f%% | %.1f%% |\n",
      $1, $2*1000, $3*1000, $4*1000, $5*1000, $6*100, $7*100}'
} > "$REPORT"

echo; cat "$REPORT"; echo; echo "Report written to $REPORT"
