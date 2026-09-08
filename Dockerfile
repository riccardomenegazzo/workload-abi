FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-X main.version=${VERSION}" -o /out/wabi ./cmd/wabi

FROM docker:cli
COPY --from=build /out/wabi /usr/local/bin/wabi
ENTRYPOINT ["wabi"]
