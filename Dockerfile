FROM alpine:3.18
VOLUME ["/data"]
RUN apk update && apk upgrade
RUN adduser -u 1000 minio -s /bin/sh -D minio

COPY --chown=minio --chmod=+x ./dist/minio /usr/local/bin/
RUN chmod +x /usr/local/bin/minio
RUN mkdir /data
RUN chown minio /data -R

USER minio
VOLUME /data
EXPOSE 9000
CMD minio
