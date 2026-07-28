FROM golang:1.25-alpine AS source

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

FROM source AS build

RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /freegent ./cmd/freegent

FROM source AS cli-build

ARG CLI_GOOS
ARG CLI_GOARCH

RUN test -n "$CLI_GOOS" \
    && test -n "$CLI_GOARCH" \
    && CGO_ENABLED=0 GOOS="$CLI_GOOS" GOARCH="$CLI_GOARCH" \
       go build -trimpath -ldflags="-s -w" -o /freegent ./cmd/freegent

FROM scratch AS cli-artifact

COPY --from=cli-build /freegent /freegent

FROM alpine:3.22 AS runtime

RUN apk add --no-cache ca-certificates \
    && mkdir -p /data/logs \
    && chown -R 65532:65532 /data

COPY --from=build /freegent /usr/local/bin/freegent

USER 65532:65532

ENTRYPOINT ["freegent"]

FROM runtime AS api

EXPOSE 8080

HEALTHCHECK --interval=5s --timeout=3s --start-period=5s --retries=20 \
  CMD wget -qO- http://localhost:8080/health >/dev/null || exit 1

CMD ["api", "-port", "8080"]

FROM runtime AS worker

HEALTHCHECK NONE

CMD ["worker"]
