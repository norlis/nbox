################################
#	Base Stage                  #
#			                    #
################################
FROM public.ecr.aws/docker/library/alpine:latest AS base
#FROM alpine:latest AS base

ARG USERNAME=app
ARG USER_UID=4317

RUN addgroup \
    -g $USER_UID \
    $USERNAME && \
    adduser \
    -D \
    -g $USERNAME \
    -h "/home/${USERNAME}"\
    -G $USERNAME \
    -u $USER_UID \
    $USERNAME

RUN apk --update add ca-certificates

################################
# TOOLS
# image used for production stage
################################
#FROM busybox:uclibc AS busybox
FROM public.ecr.aws/docker/library/busybox:stable-uclibc AS busybox


################################
#	Copy Stage             #
#			       #
################################
FROM scratch AS pre

COPY --from=busybox /bin/sh /bin/ls /bin/wget /bin/cat /bin/vi /bin/cp /bin/grep /bin/ln /bin/mkdir /bin/ps /bin/chmod /bin/

RUN /bin/wget -O opa https://openpolicyagent.org/downloads/latest/opa_linux_arm64_static && \
    /bin/chmod +x opa

################################
#	Final Stage            #
#			       #
################################
FROM pre AS production

ARG USERNAME=app

COPY --from=base /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=base /etc/passwd /etc/passwd
COPY --from=base /etc/group /etc/group
COPY --from=base /home/$USERNAME/ /home/$USERNAME

COPY authz /data
COPY ./config.yaml /.
VOLUME /data

ENV RUN_IN_CONTAINER="True"
USER $USERNAME

#CMD ["./opa", "run", "-s", "./data", "--disable-telemetry"]
CMD ["./opa", "run", "--server", "--config-file=/config.yaml", "./data", "--disable-telemetry"]
EXPOSE 8181