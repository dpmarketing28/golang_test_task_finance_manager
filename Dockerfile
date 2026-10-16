FROM golang:1.27.2-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /out/app ./cmd/app

FROM alpine:3.20.10
# ca-certificates is required for outbound HTTPS calls (Telegram Bot API,
# the configured LLM provider) — a bare alpine image has no root certs.
RUN apk add --no-cache ca-certificates && adduser -D -H app
COPY --from=build /out/app /usr/local/bin/app
USER app
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/app"]
