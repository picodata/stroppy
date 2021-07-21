FROM golang:1.16.2-stretch
ARG FDB_VERSION=6.2.28

WORKDIR /tmp
# dnsutils is needed to have dig installed to create cluster file
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates dnsutils
RUN wget --no-check-certificate "https://foundationdb.org/downloads/${FDB_VERSION}/ubuntu/installers/foundationdb-clients_${FDB_VERSION}-1_amd64.deb"
RUN dpkg -i foundationdb-clients_${FDB_VERSION}-1_amd64.deb
