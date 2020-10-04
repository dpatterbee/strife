FROM golang:1.15.2-alpine AS build

WORKDIR /go/src/strife

COPY go.mod go.sum ./
RUN go mod download -x
COPY . .
RUN go build -v -o /out/strife .

FROM alpine AS bin
RUN apk add ffmpeg
COPY creds.yml .
COPY --from=build /out/strife .
CMD ["/strife"]
