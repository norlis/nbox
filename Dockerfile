# syntax=docker/dockerfile:1.24

# BUILDMODE=build compiles from source; BUILDMODE=copy uses a pre-built binary.
#
# Targets (--target):
#   nbox        → HTTP REST API (:7337) + cli  [default]
#   entrypushd  → gRPC KVStream + SQS consumer (:9337)
ARG BUILDMODE=build

# nonroot identity (65532 = distroless/k8s runAsNonRoot convention)
ARG USERNAME=nonroot
ARG USER_UID=65532
ARG USER_GID=65532


# --- base: nonroot user + CA certs ---
FROM public.ecr.aws/docker/library/alpine:latest AS base

ARG USERNAME
ARG USER_UID
ARG USER_GID

RUN addgroup \
    -g ${USER_GID} \
    ${USERNAME} && \
    adduser \
    -D \
    -g ${USERNAME} \
    -h "/home/${USERNAME}"\
    -G ${USERNAME} \
    -u ${USER_UID} \
    ${USERNAME}

RUN apk --update add ca-certificates


# --- build from source ---
FROM public.ecr.aws/docker/library/golang:1.26 AS prep-build

ARG TARGETARCH

WORKDIR /workspace
COPY go.mod .
COPY go.sum .
RUN --mount=type=cache,target=/go/pkg/mod/ \
    go mod download -x

COPY . .

RUN --mount=type=cache,target=/go/pkg/mod/ \
    make ${TARGETARCH}-build

# microservice binary is published as `nbox`
RUN mv /workspace/build/linux/${TARGETARCH}/microservice /workspace/nbox
RUN mv /workspace/build/linux/${TARGETARCH}/entrypushd /workspace/entrypushd
RUN mv /workspace/build/linux/${TARGETARCH}/cli /workspace/cli


# --- copy pre-built binaries ---
FROM scratch AS prep-copy

WORKDIR /workspace

ARG TARGETARCH

COPY build/linux/${TARGETARCH}/microservice /workspace/nbox
COPY build/linux/${TARGETARCH}/entrypushd /workspace/entrypushd
COPY build/linux/${TARGETARCH}/cli /workspace/cli


# --- pick build or copy ---
FROM prep-${BUILDMODE} AS package


# --- shell tools for healthchecks/debug ---
FROM public.ecr.aws/docker/library/busybox:stable-uclibc AS busybox


# --- runtime base: certs, nonroot user, shell tools (shared by all services) ---
FROM scratch AS runtime-base

ARG USERNAME
ARG USER_UID
ARG USER_GID

COPY --from=base /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=base /etc/passwd /etc/passwd
COPY --from=base /etc/group /etc/group
COPY --from=base /home/${USERNAME}/ /home/${USERNAME}
COPY --from=busybox /bin/sh /bin/ls /bin/wget /bin/

ENV RUN_IN_CONTAINER="True"
# aws-sdk-go needs $HOME to find shared credentials
ENV HOME=/home/${USERNAME}

# binaries and policies live under the nonroot home; OPA reads policies/authz relative to CWD
WORKDIR /home/${USERNAME}
USER ${USER_UID}:${USER_GID}


# --- entrypushd: gRPC KVStream + SQS consumer (:9337) ---
FROM runtime-base AS entrypushd

COPY --from=package /workspace/entrypushd ./entrypushd
COPY --from=busybox /bin/nc /bin/

ENTRYPOINT ["./entrypushd"]
EXPOSE 9337


# --- nbox: HTTP REST API (:7337) + cli  [default target] ---
FROM runtime-base AS nbox

COPY --from=package /workspace/nbox ./nbox
COPY --from=package /workspace/cli /bin/cli
COPY ./policies ./policies

ENTRYPOINT ["./nbox"]
CMD ["--port=7337", "--address=0.0.0.0"]
EXPOSE 7337