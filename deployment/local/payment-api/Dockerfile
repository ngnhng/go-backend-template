# syntax=docker/dockerfile:1.7

FROM golang:1.25-alpine AS build
WORKDIR /src
RUN apk add --no-cache build-base git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/payment-api .

FROM alpine:3.20 AS app-local
RUN addgroup -S app && adduser -S app -G app
WORKDIR /srv/payment-api
COPY --from=build /out/payment-api ./payment-api
USER app
EXPOSE 8080
ENTRYPOINT ["./payment-api"]
