FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
ARG VERSION=container
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/wabi ./cmd/wabi

FROM alpine:3.20
RUN apk add --no-cache ca-certificates docker-cli docker-cli-compose
COPY --from=build /out/wabi /usr/local/bin/wabi
ENTRYPOINT ["wabi"]
