FROM golang:alpine AS build

WORKDIR /go/src/strife
ENV CGO_ENABLED=0
COPY go.mod go.sum ./
RUN go mod download -x
COPY *.go ./
COPY ./src/*.go ./src/
COPY ./bufferedpipe/*.go ./bufferedpipe/
RUN go build -v -o /out/strife .

FROM alpine AS bin
RUN apk add ffmpeg
COPY creds.yml .
COPY --from=build /out/strife .
CMD ["./strife"]
