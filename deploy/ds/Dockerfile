# 389 Directory Server on Debian, container layout compatible with upstream dscontainer:
# all state lives in /data, LDAPI socket at /data/run/slapd-localhost.socket.
FROM debian:trixie-slim

# Fixed uid/gid so that sidecar containers (ds-ui) can create files owned by dirsrv.
ARG DS_UID=389
ARG DS_GID=389

RUN groupadd --system --gid "${DS_GID}" dirsrv \
 && useradd --system --uid "${DS_UID}" --gid dirsrv --home-dir /data --no-create-home \
      --shell /usr/sbin/nologin dirsrv \
 && apt-get update \
 && apt-get install -y --no-install-recommends 389-ds-base tini ca-certificates \
 && rm -rf /var/lib/apt/lists/*

# dscontainer expects config/ssca to be symlinked into /data (see its --help).
RUN mkdir -p /data/config /data/ssca /data/run \
 && rm -rf /etc/dirsrv/slapd-localhost /etc/dirsrv/ssca \
 && ln -s /data/config /etc/dirsrv/slapd-localhost \
 && ln -s /data/ssca /etc/dirsrv/ssca \
 && chown -R dirsrv:dirsrv /data

COPY --chmod=0755 entrypoint.sh /usr/local/bin/ds-entrypoint

USER dirsrv
VOLUME /data
EXPOSE 3389 3636

HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 \
  CMD ["/usr/libexec/dirsrv/dscontainer", "-H"]

ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/ds-entrypoint"]
