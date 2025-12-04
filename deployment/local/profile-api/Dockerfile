FROM golang:1.25.1-alpine AS builder
WORKDIR /app

COPY go.mod go.sum ./
COPY vendor ./vendor

COPY . .
ARG TARGETOS TARGETARCH
ENV CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH GOFLAGS=-mod=vendor
RUN go build -trimpath -ldflags="-s -w" -o /app/modules/profile-api .

FROM gcr.io/distroless/static-debian12 AS app-local
USER nonroot:nonroot
COPY --from=builder /app/modules/profile-api /profile-api
EXPOSE 8080
ENTRYPOINT ["/profile-api"]
