# base stage
FROM golang:alpine AS base

RUN apk add --update git ca-certificates build-base bash && \
    go get -u github.com/Masterminds/glide

RUN ln -sf /bin/bash /bin/sh

WORKDIR $GOPATH/src/github.com/shellhub-io/shellhub/agent

COPY glide.yaml glide.lock ./

RUN glide install

# builder stage
FROM base AS builder

COPY . .

RUN glide -q install --skip-test && go build

# development stage
FROM base AS development

RUN apk add --update openssl openssh-client
RUN go get github.com/markbates/refresh

RUN glide -q install --skip-test
RUN cp -a vendor /vendor

COPY entrypoint-dev.sh /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]

# production stage
FROM alpine:3.7 AS production

WORKDIR /app
COPY --from=builder /agent /app/

ENTRYPOINT ./agent
