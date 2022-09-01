FROM docker.binary.picodata.io/stroppy-builder:latest
WORKDIR /stroppy
USER root:root
COPY . .
RUN go build -o bin/stroppy ./cmd/stroppy


FROM docker-proxy.binary.picodata.io/debian:bullseye-slim
ARG COMMIT_HASH \
    FDB_VERSION=6.3.23 \
    TF_VERSION=1.2.5 \
    KCTL_VERSION=v1.24.0
ENV COMMIT_HASH ${COMMIT_HASH:-manual_build} \
    PATH=$PATH:~/.local/bin
RUN useradd -m stroppy
    #&& usermod -aG sudo stroppy
COPY third_party /home/stroppy/third_party
COPY build/.terraformrc /home/stroppy/.terraformrc
RUN apt update \
    && apt install -y ca-certificates wget curl python3 pip unzip ssh vim \
    && apt clean
RUN pip install -r /home/stroppy/third_party/kubespray/requirements.txt \
    && curl -fsLO https://github.com/apple/foundationdb/releases/download/${FDB_VERSION}/foundationdb-clients_${FDB_VERSION}-1_amd64.deb \
    && dpkg -i foundationdb-clients_${FDB_VERSION}-1_amd64.deb \
    && curl -fsLO https://binary.picodata.io/repository/raw/terraform/terraform_${TF_VERSION}_amd64.deb \
    && dpkg -i terraform_1.2.5_amd64.deb \
    && curl -fsLO https://storage.googleapis.com/kubernetes-release/release/${KCTL_VERSION}/bin/linux/amd64/kubectl \
    && chmod +x kubectl && mv kubectl /usr/bin/kubectl \
    && rm -f foundationdb-clients_${FDB_VERSION}-1_amd64.deb terraform_${TF_VERSION}_amd64.deb \
    && chown -R stroppy:stroppy /home/stroppy
COPY --from=0 stroppy/bin/stroppy /usr/local/bin/stroppy
USER stroppy:stroppy
WORKDIR /home/stroppy
CMD ["/bin/bash"]
