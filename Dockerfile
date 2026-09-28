# Keep this in step with the go directive in go.mod: the official golang
# images set GOTOOLCHAIN=local, so an older image refuses to build.
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /netpulse ./cmd/netpulse

# nonroot (uid 65532) still gets unprivileged ICMP: Docker sets
# net.ipv4.ping_group_range to "0 2147483647" inside containers.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /netpulse /netpulse
EXPOSE 9101
ENTRYPOINT ["/netpulse"]
