FROM golang:1.24.3-alpine3.22 AS build

COPY . /go/src/linx-server
WORKDIR /go/src/linx-server

RUN set -ex \
        && apk add --no-cache --virtual .build-deps git \
        && go build \
        && apk del .build-deps

FROM alpine:3.22

COPY --from=build /go/src/linx-server/linx-server /usr/local/bin/linx-server

RUN mkdir -p /data/files && mkdir -p /data/meta && chown -R 65534:65534 /data

VOLUME ["/data/files", "/data/meta"]

EXPOSE 8080
USER nobody
ENTRYPOINT ["/usr/local/bin/linx-server", "-bind=0.0.0.0:8080", "-filespath=/data/files/", "-metapath=/data/meta/"]
CMD ["-sitename=linx", "-allowhotlink"]
