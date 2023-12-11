FROM alpine:latest
VOLUME ["/data"]
RUN apk update && apk upgrade
RUN addgroup -g 1000 minio\
    && adduser -u 1000 minio -s /bin/sh -D minio

COPY ./dist/minio /usr/bin/minio
RUN chmod +x /usr/bin/minio

USER minio
EXPOSE 9000
EXPOSE 9001

CMD minio server ~/minio --console-address :9001
