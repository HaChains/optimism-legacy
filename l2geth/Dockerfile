# Build Geth in a stock Go builder container
FROM alpine:3.15 as environment
RUN set -x \
    && echo "http://mirrors.huaweicloud.com/alpine/latest-stable/main/" > /etc/apk/repositories \
    && echo "http://mirrors.huaweicloud.com/alpine/latest-stable/community/" >> /etc/apk/repositories \
    && apk update \
    && apk add --no-cache ca-certificates jq curl

FROM golang:1.18.0-alpine3.15 as builder

RUN set -x \
    && echo "http://mirrors.huaweicloud.com/alpine/latest-stable/main/" > /etc/apk/repositories \
    && echo "http://mirrors.huaweicloud.com/alpine/latest-stable/community/" >> /etc/apk/repositories \
    && apk update \
    && apk add --no-cache make gcc musl-dev linux-headers git

WORKDIR /app/l2geth
COPY go.mod /app/l2geth
COPY go.sum /app/l2geth
RUN go env -w GOPROXY=https://goproxy.cn,direct && go mod download

COPY . /app/l2geth
RUN make geth

# Pull Geth into a second stage deploy alpine container
FROM environment
COPY --from=builder /app/l2geth/build/bin/geth /usr/local/bin/

WORKDIR /usr/local/bin/
EXPOSE 8545 8546 8547
COPY ./geth.sh .
ENTRYPOINT ["geth"]
