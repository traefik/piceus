FROM alpine

RUN apk --no-cache --no-progress add git ca-certificates tzdata make \
    && update-ca-certificates \
    && rm -rf /var/cache/apk/*

COPY ./dist/linux/amd64/piceus .

ENTRYPOINT ["/piceus"]
EXPOSE 80
