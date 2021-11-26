FROM amazonlinux:2.0.20210721.2

RUN yum -y install shadow-utils \
    && groupadd abill \
    && useradd -g abill abill \
    && yum clean all \
    && rm -rf /var/cache/yum

COPY --chown=abill:abill build/ab-bsn-amd64 /app/
COPY --chown=abill:abill cmd/bsn/config.yaml /app/

WORKDIR /app
USER abill
ENV PATH="/app:${PATH}"
ENTRYPOINT ["ab-bsn", "-config_path=config.yaml"]
