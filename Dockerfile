FROM alpine:3.18
VOLUME ["/data"]
RUN apk update && apk upgrade
RUN adduser -u 1000 minio -s /bin/sh -D minio

USER minio
COPY ./dist/minio /usr/bin/minio
EXPOSE 9000
EXPOSE 9001

CMD minio server ~/minio --console-address :9001
