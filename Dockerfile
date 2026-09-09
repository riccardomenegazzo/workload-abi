FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
ARG VERSION=container
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/wabi ./cmd/wabi

FROM alpine:3.22
RUN apk add --no-cache ca-certificates docker-cli docker-cli-compose
COPY --from=build /out/wabi /usr/local/bin/wabi
ENTRYPOINT ["wabi"]
