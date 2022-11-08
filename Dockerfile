FROM golang:1.19-alpine as builder

WORKDIR /app

COPY . .

RUN go mod download && go build -o ./pod-as-resource ./cmd/...

FROM alpine:3.16.2

RUN apk add --no-cache bash

COPY --from=builder /app/pod-as-resource /pod-as-resource
COPY examples/example-config.yaml /etc/pod-as-resource/config.yaml


ENTRYPOINT [ "/pod-as-resource" ]

