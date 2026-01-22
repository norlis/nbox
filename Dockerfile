# The build mode (options: build, copy) passed in as a --build-arg. If build is specified, then the copy
# stage will be skipped and vice versa. The build mode builds the binary from the source files, while
# the copy mode copies in a pre-built binary.
ARG BUILDMODE=build


################################
#	Base Stage                  #
#			                    #
################################
FROM public.ecr.aws/docker/library/alpine:latest AS base
#FROM alpine:latest AS base

ARG USERNAME=app
ARG USER_UID=4317

RUN addgroup \
    -g ${USER_UID} \
    ${USERNAME} && \
    adduser \
    -D \
    -g ${USERNAME} \
    -h "/home/${USERNAME}"\
    -G ${USERNAME} \
    -u ${USER_UID} \
    ${USERNAME}

RUN apk --update add ca-certificates


################################
#	Build Stage            #
#			       #
################################
#FROM golang:1.22 AS prep-build
FROM public.ecr.aws/docker/library/golang:1.25 AS prep-build

ARG TARGETARCH

# download go modules ahead to speed up the building
WORKDIR /workspace
COPY go.mod .
COPY go.sum .
RUN --mount=type=cache,target=/go/pkg/mod/ \
    go mod download -x

# copy source
COPY . .

# build
RUN --mount=type=cache,target=/go/pkg/mod/ \
    make ${TARGETARCH}-build

# move
RUN mv /workspace/build/linux/${TARGETARCH}/microservice /workspace/microservice
RUN mv /workspace/build/linux/${TARGETARCH}/hasher /workspace/hasher



################################
#	Copy Stage             #
#			       #
################################
FROM scratch AS prep-copy

WORKDIR /workspace

ARG TARGETARCH

# copy artifacts
# always assume binary is created
COPY build/linux/${TARGETARCH}/microservice /workspace/microservice
COPY build/linux/${TARGETARCH}/hasher /workspace/hasher


################################
#	Packing Stage          #
#			       #
################################
FROM prep-${BUILDMODE} AS package

#COPY application.yaml /workspace/application.yaml


################################
# TOOLS
# image used for production stage
################################
#FROM busybox:uclibc AS busybox
FROM public.ecr.aws/docker/library/busybox:stable-uclibc AS busybox


################################
#	Final Stage            #
#			       #
################################
FROM scratch  AS production

ARG USERNAME=app

COPY --from=base /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=base /etc/passwd /etc/passwd
COPY --from=base /etc/group /etc/group
COPY --from=base /home/$USERNAME/ /home/$USERNAME
COPY --from=package /workspace/microservice /microservice
COPY --from=package /workspace/hasher /bin/hasher
COPY ./policies /policies

COPY --from=busybox /bin/sh /bin/ls /bin/wget /bin/cat /bin/vi /bin/cp /bin/grep /bin/ln /bin/mkdir /bin/ps /bin/

ENV RUN_IN_CONTAINER="True"

USER $USERNAME
# aws-sdk-go needs $HOME to look up shared credentials
ENV HOME=/home/$USERNAME
ENTRYPOINT ["/microservice"]
CMD ["--port=7337", "--address=0.0.0.0"]

EXPOSE 7337
