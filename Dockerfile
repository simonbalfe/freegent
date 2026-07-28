FROM golang:1.25-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /freegent ./cmd/freegent

FROM alpine:3.22

RUN apk add --no-cache ca-certificates \
    && mkdir -p /data/logs \
    && chown -R 65532:65532 /data

COPY --from=build /freegent /usr/local/bin/freegent

USER 65532:65532

EXPOSE 8080

HEALTHCHECK --interval=5s --timeout=3s --start-period=5s --retries=20 \
  CMD wget -qO- http://localhost:8080/health >/dev/null || exit 1

ENTRYPOINT ["freegent"]
CMD ["api", "-port", "8080"]
