FROM golang:1.25.11-bookworm
RUN apt-get update && apt-get install -y --no-install-recommends gcc-arm-linux-gnueabihf libc6-dev-armhf-cross cmake pkg-config ca-certificates curl python3 fonts-wqy-microhei && rm -rf /var/lib/apt/lists/*
WORKDIR /work
