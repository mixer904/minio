FROM alpine:3.18
VOLUME ["/data"]
RUN apk update && apk upgrade
RUN adduser -u 1000 minio -s /bin/sh -D minio

COPY ./dist/minio /usr/local/bin/minio
RUN chmod +x /usr/local/bin/minio
RUN chown minio /usr/local/bin/minio
RUN ls -al /usr/local/bin/minio

USER minio
RUN ls -al /usr/local/bin/minio
EXPOSE 9000
VOLUME /data
CMD minio
