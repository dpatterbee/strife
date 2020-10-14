FROM golang:1.15.2-alpine AS build

WORKDIR /go/src/strife
ENV CGO_ENABLED=0
COPY go.mod go.sum ./
RUN go mod download -x
COPY *.go ./
COPY ./src/*.go ./src/
RUN go build -v -o /out/strife .

FROM python:3.9-alpine AS bin
RUN apk add ffmpeg
RUN wget https://yt-dl.org/downloads/latest/youtube-dl -O /usr/local/bin/youtube-dl
RUN chmod a+rx /usr/local/bin/youtube-dl
COPY creds.yml .
COPY --from=build /out/strife .
CMD ["./strife"]
