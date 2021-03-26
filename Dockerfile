FROM golang:1.16-alpine3.12 AS builder

WORKDIR /app

COPY main.go /app/

RUN go build main.go

FROM alpine:3.12

WORKDIR /app/

COPY ./web /app/web/
COPY --from=builder /app/main /app/generate

CMD ["/app/generate"]