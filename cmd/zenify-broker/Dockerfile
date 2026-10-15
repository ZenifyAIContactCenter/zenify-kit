# Built by GoReleaser (dockers): the context holds the prebuilt zenify-broker binary.
ARG MCP_GRAFANA_VERSION=2.0.2
FROM grafana/mcp-grafana:${MCP_GRAFANA_VERSION} AS mcp-grafana

FROM node:22-bookworm-slim
ARG MONGOSH_VERSION=2.13.0
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates default-mysql-client \
 && rm -rf /var/lib/apt/lists/* \
 && npm install -g "mongosh@${MONGOSH_VERSION}" \
 && mongosh --version && mysql --version

COPY --from=mcp-grafana /app/mcp-grafana /usr/local/bin/mcp-grafana
# Fail the build if a flag the broker passes is gone from this mcp-grafana.
RUN mcp-grafana --help 2>&1 | tee /tmp/mcp-help.txt >/dev/null; \
    for f in -t --address --disable-write --enabled-tools --loki-guardrail-mode \
             --max-loki-log-limit --loki-guardrail-max-range --usage-stats; do \
      grep -q -- "$f" /tmp/mcp-help.txt || { echo "mcp-grafana --help lacks $f"; exit 1; }; \
    done; rm /tmp/mcp-help.txt

COPY zenify-broker /usr/local/bin/zenify-broker
USER node
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/zenify-broker"]
CMD ["--config", "/etc/zenify-broker/broker.yaml"]
