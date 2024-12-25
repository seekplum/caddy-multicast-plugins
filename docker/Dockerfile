ARG VERSION

FROM caddy:${VERSION}-builder AS builder

ENV GOPRIVATE xxx.com
ENV GOPROXY https://goproxy.io/

RUN mkdir /app
ADD go.mod /app
ADD go.sum /app

# 提前构建，加速docker build 缓存
RUN xcaddy build

ADD multicast.go /app
RUN xcaddy build \
    --with github.com/seekplum/caddy-multicast-plugins=/app/

FROM caddy:${VERSION}

COPY --from=builder /usr/bin/caddy /usr/bin/caddy
