# Multi-Stage-Build: statisches Go-Binary (CGO-frei dank modernc.org/sqlite).
FROM golang:1.27@sha256:f20d83a83e18dbd2a4714b8ffd4b30b287b6243962dd75608e86abcc63ba4efc AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/server /server
# SQLite-Daten liegen auf einem PVC unter /data.
VOLUME /data
EXPOSE 8080
ENTRYPOINT ["/server"]
