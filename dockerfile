FROM golang:1.23 AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/flash-go .

FROM isolate:latest

WORKDIR /app
RUN apt-get update \
  && apt-get install -y --no-install-recommends python3 \
  && rm -rf /var/lib/apt/lists/*

COPY --from=builder /out/flash-go /app/flash-go

ENV PORT=3001
EXPOSE 3001

ENTRYPOINT ["/app/flash-go"]
