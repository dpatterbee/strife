#syntax=docker/dockerfile:1.2
FROM golang:alpine AS build

WORKDIR /go/src/strife
RUN apk add build-base
COPY . /src/strife
WORKDIR /src/strife
RUN --mount=type=cache,target=/root/.cache/go-build \
	--mount=type=cache,target=/go/pkg \
    go build -v -o /out/strife .

ADD https://github.com/benbjohnson/litestream/releases/download/v0.3.4/litestream-v0.3.4-linux-amd64-static.tar.gz /tmp/litestream.tar.gz
RUN tar -C /usr/local/bin -xzf /tmp/litestream.tar.gz

FROM alpine AS bin

ARG LITESTREAM_ACCESS_KEY_ID
ARG LITESTREAM_SECRET_ACCESS_KEY
ARG DB_REPLICA_URL
ARG TOKEN

RUN apk add ffmpeg
ENV DB_PATH=data/store.db
ENV LITESTREAM_ACCESS_KEY_ID=${LITESTREAM_ACCESS_KEY_ID}
ENV LITESTREAM_SECRET_ACCESS_KEY=${LITESTREAM_SECRET_ACCESS_KEY}
ENV DB_REPLICA_URL=${DB_REPLICA_URL}
ENV TOKEN=${TOKEN}
COPY litestream.yml /etc/litestream.yml
COPY docker_entrypoint.sh /app/
COPY restart_bot.sh /app/
COPY --from=build /out/strife /app/
COPY --from=build /usr/local/bin/litestream /usr/local/bin/litestream
CMD ["/app/docker_entrypoint.sh"]
