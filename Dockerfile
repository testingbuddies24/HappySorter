# syntax=docker/dockerfile:1

FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/happy-sorter ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -g 1000 happysorter \
    && adduser -D -u 1000 -G happysorter happysorter
COPY --from=build /out/happy-sorter /usr/local/bin/happy-sorter
USER happysorter
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/happy-sorter"]
