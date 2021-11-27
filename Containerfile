FROM scratch
ARG BUILT_ARTIFACT
ADD "$BUILT_ARTIFACT" /
ADD https://raw.githubusercontent.com/bagder/ca-bundle/master/ca-bundle.crt /etc/ssl/certs/ca-certificates.crt
EXPOSE 8080
ENTRYPOINT [ "/photo-bot" ]
CMD []

