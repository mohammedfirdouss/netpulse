#!/usr/bin/env bash
# Measure BGP reconvergence when the direct r1-r3 link silently fails.
#
#   ./convergence.sh [runs_per_mode] [modes...]
#   ./convergence.sh 3 default tuned bfd
#
# Modes:
#   default  FRR traditional timers: keepalive 60s, hold 180s
#   tuned    keepalive 3s, hold 9s
#   bfd         default BGP timers + BFD (300ms x 3 = 900ms detection), FRR's
#               default reaction: bgpd waits a 30s "BFD hold time" before
#               tearing the session down
#   bfd-strict  as bfd, with `bfd strict hold-time 1`: tear down after 1s
#
# The link is failed with `netem loss 100%` on both ends, NOT `ip link down`.
# Taking the interface down triggers FRR's fast-external-failover, which
# drops the eBGP session instantly regardless of timers, and a veth pair
# takes the far end down too. A blackhole keeps carrier up, so only
# keepalives or BFD can notice, which is the thing being measured.
#
# Ground truth is fping from h1 to h2 every 20ms: outage = longest time
# between consecutive replies. fping sends on a fixed schedule; iputils
# ping backs off while replies are outstanding, so during an outage it
# stops sending and can't see when the path comes back. NetPulse's own 1s
# probes are reported alongside.
#
# Runs inside the Linux VM (needs docker, nsenter, tc, jq).
set -euo pipefail
cd "$(dirname "$0")"

RUNS=${1:-3}; shift || true
if (( $# )); then MODES=("$@"); else MODES=(default tuned bfd bfd-strict); fi
PING_MS=20
LAB=clab-bgp

# Neighbours per router: "asn peer1 peer2"
declare_peers() {
  case $1 in
    r1) echo "65001 10.0.12.2 10.0.13.2" ;;
    r2) echo "65002 10.0.12.1 10.0.23.2" ;;
    r3) echo "65003 10.0.23.1 10.0.13.1" ;;
  esac
}

vty() { docker exec "$LAB-$1" vtysh "${@:2}"; }

set_mode() {
  local mode=$1 r asn peers p cmd cmds
  for r in r1 r2 r3; do
    read -r asn peers <<<"$(declare_peers $r)"
    cmds=()
    for p in $peers; do
      # Fast reconnect once the link heals; doesn't affect failure detection.
      cmds+=("neighbor $p timers connect 5")
      case $mode in
        default)    cmds+=("no neighbor $p timers" "no neighbor $p bfd") ;;
        tuned)      cmds+=("neighbor $p timers 3 9" "no neighbor $p bfd") ;;
        bfd)        cmds+=("no neighbor $p timers" "no neighbor $p bfd strict" "neighbor $p bfd") ;;
        bfd-strict) cmds+=("no neighbor $p timers" "neighbor $p bfd" "neighbor $p bfd strict hold-time 1") ;;
      esac
    done
    # One vtysh call per command: vtysh stops at the first failing -c, and
    # "no ..." fails when the setting is already absent.
    for cmd in "${cmds[@]}"; do
      vty $r -c "configure terminal" -c "router bgp $asn" -c "$cmd" >/dev/null 2>&1 || true
    done
    # Timers are negotiated in OPEN, so sessions must restart to pick them up.
    vty $r -c "clear bgp ipv4 unicast *" >/dev/null
  done
}

link() { # up|down on r1:eth2 and r3:eth2
  local r pid
  for r in r1 r3; do
    pid=$(docker inspect -f '{{.State.Pid}}' "$LAB-$r")
    if [[ $1 == down ]]; then
      nsenter -t "$pid" -n tc qdisc replace dev eth2 root netem loss 100%
    else
      nsenter -t "$pid" -n tc qdisc del dev eth2 root 2>/dev/null || true
    fi
  done
}

established() { # all 6 sessions up
  local r n=0
  for r in r1 r2 r3; do
    n=$(( n + $(vty $r -c "show bgp ipv4 summary json" |
      jq 2>/dev/null '[(.ipv4Unicast.peers // {})[] | select(.state=="Established")] | length') ))
  done
  [[ $n -eq 6 ]]
}

primary_path() { # r1 reaches h2 via the direct link
  vty r1 -c "show ip route 192.168.3.0/24 json" |
    jq -e 2>/dev/null '.["192.168.3.0/24"][0].nexthops[] | select(.ip=="10.0.13.2" and .active)' >/dev/null
}

wait_for() { # timeout_s description cmd...
  local t=$1 what=$2; shift 2
  for ((i = 0; i < t; i++)); do "$@" && return 0; sleep 1; done
  echo "timed out waiting for $what" >&2; return 1
}

netpulse_lost() {
  docker exec "$LAB-h1" curl -s localhost:9101/metrics |
    awk '/^netpulse_packets_sent_total/ {s=$2} /^netpulse_packets_received_total/ {r=$2} END {print s-r}'
}

# Hold time with the timer phase at its worst, plus headroom.
hold_window() {
  case $1 in default) echo 200 ;; tuned) echo 20 ;; bfd) echo 45 ;; bfd-strict) echo 10 ;; esac
}

mkdir -p ../../results
REPORT=../../results/$(date +%Y%m%d-%H%M%S)-bgp.md
RAW=$(mktemp)
trap 'link up' EXIT

for mode in "${MODES[@]}"; do
  echo "=== mode: $mode"
  link up
  set_mode "$mode"
  wait_for 120 "sessions" established
  wait_for 60 "primary path" primary_path
  sleep 5

  for ((run = 1; run <= RUNS; run++)); do
    window=$(hold_window "$mode")
    np_before=$(netpulse_lost)
    pingout=$(mktemp)
    docker exec "$LAB-h1" fping -D -p "$PING_MS" -t 100 -c $(( (window + 10) * 1000 / PING_MS )) \
      192.168.3.10 >"$pingout" 2>/dev/null &
    ping_pid=$!
    sleep 5
    fail_at=$(date -u +%H:%M:%S)
    link down
    sleep "$window"
    link up
    wait "$ping_pid" || true
    np_lost=$(( $(netpulse_lost) - np_before ))

    # Longest silence between consecutive replies. If replies never came
    # back, the silence runs to when ping stopped.
    ping_end=$(date +%s.%N)
    outage=$(awk -v i="$(awk -v m=$PING_MS 'BEGIN { print m / 1000 }')" -v end="$ping_end" '
      /^\[[0-9.]+\] .* bytes,/ { t = substr($1, 2, length($1) - 2) + 0
        if (n++ && t - prev > max) max = t - prev; prev = t }
      END { if (end - prev > max) max = end - prev; printf "%.2f", max - i }' "$pingout")
    echo "  run $run: failed at $fail_at UTC, outage ${outage}s (fping), NetPulse lost ${np_lost} x 1s probes"
    printf '%s\t%s\t%s\t%s\t%s\n' "$mode" "$run" "$fail_at" "$outage" "$np_lost" >>"$RAW"
    rm -f "$pingout"

    # Heal and let the direct path come back before the next run.
    wait_for 120 "sessions" established
    wait_for 60 "primary path" primary_path
    sleep 5
  done
done

set_mode default >/dev/null 2>&1 || true

{
  echo "# BGP convergence: silent failure of the r1-r3 link"
  echo
  echo "- Date: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "- Failure: \`netem loss 100%\` on r1:eth2 and r3:eth2 (carrier stays up)"
  echo "- Outage measured by fping h1 -> h2 every ${PING_MS}ms (longest gap between replies)"
  echo
  echo "## Per run"
  echo
  echo "| Mode | Run | Failed at (UTC) | Outage (fping) | NetPulse probes lost (1s) |"
  echo "|---|---|---|---|---|"
  awk -F'\t' '{ printf "| %s | %s | %s | %ss | %s |\n", $1, $2, $3, $4, $5 }' "$RAW"
  echo
  echo "## Summary"
  echo
  echo "| Mode | Min | Mean | Max |"
  echo "|---|---|---|---|"
  awk -F'\t' '
    { n[$1]++; s[$1] += $4; if (!($1 in mn) || $4 < mn[$1]) mn[$1] = $4; if ($4 > mx[$1]) mx[$1] = $4
      if (!($1 in seen)) { seen[$1] = 1; order[++k] = $1 } }
    END { for (i = 1; i <= k; i++) { m = order[i]; printf "| %s | %.2fs | %.2fs | %.2fs |\n", m, mn[m], s[m] / n[m], mx[m] } }' "$RAW"
} >"$REPORT"

echo; cat "$REPORT"; echo "Report written to $REPORT"
