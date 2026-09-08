FROM golang:1.23-alpine AS build
WORKDIR /src
COPY main.go .
RUN CGO_ENABLED=0 go build -trimpath -o /out/app main.go

FROM scratch
COPY --from=build /out/app /app
EXPOSE 8080
ENTRYPOINT ["/app"]
