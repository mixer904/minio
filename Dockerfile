FROM alpine:3.18
VOLUME ["/data"]
RUN apk update && apk upgrade
RUN adduser -u 1000 minio -s /bin/sh -D minio

COPY --chown=minio --chmod=+x ./dist/minio /usr/local/bin/
RUN chmod +x /usr/local/bin/minio

USER minio
EXPOSE 9000
VOLUME /data
CMD minio
