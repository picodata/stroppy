FROM docker.binary.picodata.io/stroppy-builder:latest
WORKDIR /stroppy
USER root:root
COPY . .
RUN go build -o bin/stroppy ./cmd/stroppy

FROM docker-proxy.binary.picodata.io/debian:bullseye-slim
ARG FDB_VERSION=6.3.23
RUN apt-get update \
    && apt-get install -y ca-certificates wget \
    && apt-get install -y curl \
    && apt-get clean
RUN wget "https://github.com/apple/foundationdb/releases/download/${FDB_VERSION}/foundationdb-clients_${FDB_VERSION}-1_amd64.deb"
RUN dpkg -i foundationdb-clients_${FDB_VERSION}-1_amd64.deb
WORKDIR /root
ENV PATH "${PATH}:/root"
ARG COMMIT_HASH
ENV COMMIT_HASH ${COMMIT_HASH:-manual_build}

#добавляем директорию с файлами для деплоя и конфигурирования stroppy
COPY --from=0 stroppy/bin/stroppy .
CMD ["/bin/bash"]

