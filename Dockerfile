FROM docker.1ms.run/library/golang:1.24.5 AS builder
ADD . /src
WORKDIR /src
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64  go build -mod vendor -o web-controller cmd/helm/main.go

FROM docker.1ms.run/library/alpine:3.22

RUN apk add --no-cache mysql-client

COPY --from=builder /src/web-controller  /opt/
RUN chmod a+x /opt/web-controller

EXPOSE 8080
EXPOSE 50051
CMD [ "/opt/web-controller" ]
