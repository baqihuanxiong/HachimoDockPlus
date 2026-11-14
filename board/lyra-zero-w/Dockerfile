FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    bc bison build-essential ca-certificates cpio device-tree-compiler \
    file flex git kmod libelf-dev libmpc-dev libncurses-dev libssl-dev lz4 \
    openssh-client python3 rsync u-boot-tools xz-utils \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /work
