FROM golang:1.14-alpine as build

WORKDIR /go/src/github.com/xtrafrancyz/vk-proxy/

COPY . .

RUN go install && go build

FROM alpine:3.12

EXPOSE 8080

ENV PORT 8080
ENV DOMAIN vk-api-proxy.example.com
ENV STATIC_DOMAIN vk-static-proxy.example.com

WORKDIR /app

COPY --from=build /go/src/github.com/xtrafrancyz/vk-proxy/vk-proxy/ /app/vk-proxy

ENTRYPOINT ["/bin/sh", "-c", "/app/vk-proxy -allowMissingConfig -bind 0.0.0.0:$PORT -domain $DOMAIN -domain-static $STATIC_DOMAIN" ]
