FROM registry.gitlab.com/picodata/dockers/stroppy_builder:v2
WORKDIR /stroppy
COPY . .
RUN go build -o bin/stroppy

FROM debian:9.13-slim
ARG FDB_VERSION=6.2.28
RUN apt-get update && apt-get install -y ca-certificates wget && apt-get install -y curl &&apt-get clean
RUN wget "https://foundationdb.org/downloads/${FDB_VERSION}/ubuntu/installers/foundationdb-clients_${FDB_VERSION}-1_amd64.deb"
RUN dpkg -i foundationdb-clients_${FDB_VERSION}-1_amd64.deb
WORKDIR /root
ENV PATH "${PATH}:/root"
ARG COMMIT_HASH
ENV COMMIT_HASH ${COMMIT_HASH:-manual_build}
#добавляем директорию с файлами для деплоя и конфигурирования stroppy
COPY --from=0 stroppy/deploy/ deploy/
COPY --from=0 stroppy/bin/stroppy .