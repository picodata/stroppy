FROM docker.binary.picodata.io/stroppy-builder:latest
WORKDIR /stroppy
USER root:root
COPY . .
RUN go build -o bin/stroppy ./cmd/stroppy

FROM docker-proxy.binary.picodata.io/debian:bullseye-slim
ARG FDB_VERSION=6.3.23
COPY third_party .
RUN apt update \
    && apt install -y ca-certificates wget curl python3 pip \
    && apt clean
RUN pip install -r third_party/kubespray/requirements.txt
RUN wget "https://github.com/apple/foundationdb/releases/download/${FDB_VERSION}/foundationdb-clients_${FDB_VERSION}-1_amd64.deb" \
    && dpkg -i foundationdb-clients_${FDB_VERSION}-1_amd64.deb
WORKDIR /root
ENV PATH "${PATH}:/root"
ARG COMMIT_HASH
ENV COMMIT_HASH ${COMMIT_HASH:-manual_build}
    PATH=$PATH:~/.local/bin

#добавляем директорию с файлами для деплоя и конфигурирования stroppy
COPY --from=0 stroppy/bin/stroppy .
CMD ["/bin/bash"]
