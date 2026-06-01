FROM --platform=$BUILDPLATFORM golang:1.26.2-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath -ldflags="-s -w" -o /out/hysterical-panel .

FROM alpine:3.21

RUN apk add --no-cache ca-certificates \
    && addgroup -S panel \
    && adduser -S -D -H -h /app -s /sbin/nologin -G panel panel \
    && mkdir -p /app/pb_data \
    && chown -R panel:panel /app

WORKDIR /app

COPY --from=build /out/hysterical-panel /usr/local/bin/hysterical-panel
COPY --from=build --chown=panel:panel /src/mmdb /app/mmdb

ENV PB_DATA_DIR=/app/pb_data

VOLUME ["/app/pb_data"]
EXPOSE 8090

USER panel

ENTRYPOINT ["hysterical-panel"]
CMD ["serve", "--http=0.0.0.0:8090"]
