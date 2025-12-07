# syntax=docker/dockerfile:1.7
# Copyright 2025 Nhat-Nguyen Nguyen
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.


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
