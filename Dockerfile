FROM golang:1.25-alpine AS source

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

FROM source AS build

RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /freegent ./cmd/freegent \
    && CGO_ENABLED=0 GOOS=darwin go build -trimpath -ldflags="-s -w" -o /freegent-darwin ./cmd/freegent

FROM alpine:3.22 AS runtime

RUN apk add --no-cache ca-certificates

COPY --from=build /freegent /usr/local/bin/freegent
COPY --from=build /freegent-darwin /opt/freegent/darwin/freegent
COPY compose.yaml .env.example SKILL.md /opt/freegent/install/

RUN mkdir -p /opt/freegent/linux \
    && ln /usr/local/bin/freegent /opt/freegent/linux/freegent

USER 65532:65532

ENTRYPOINT ["freegent"]

FROM runtime AS api

EXPOSE 8080

HEALTHCHECK --interval=5s --timeout=3s --start-period=5s --retries=20 \
  CMD wget -qO- http://localhost:8080/health >/dev/null || exit 1

CMD ["api", "-port", "8080"]
