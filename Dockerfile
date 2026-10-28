# syntax=docker/dockerfile:1

FROM golang:1.26-alpine AS build
WORKDIR /src
RUN go install github.com/swaggo/swag/cmd/swag@v1.16.6
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN swag init -g cmd/api/main.go -o api --parseInternal --outputTypes go,json,yaml --packageName api \
    && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/taskservice ./cmd/api

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -H -u 10001 app
USER app
COPY --from=build /out/taskservice /usr/local/bin/taskservice
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/taskservice"]
