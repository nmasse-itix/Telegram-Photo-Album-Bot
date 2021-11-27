FROM alpine:latest
RUN apk add -U --no-cache ca-certificates
ARG BUILT_ARTIFACT
ADD "$BUILT_ARTIFACT" /
EXPOSE 8080
ENTRYPOINT [ "/photo-bot" ]
CMD []

