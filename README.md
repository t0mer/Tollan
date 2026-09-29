# Tollan

**Tollan** is a self-hosted log management server in a single Go binary (or a
scratch-based Docker image<!-- TODO: verify image size (previously stated as ~17 MB) -->).
It ingests logs over many protocols, normalizes them through pipelines, routes
them into streams, stores them in a searchable time-series log store, and exposes
search, dashboards, alerting, outputs and role-based access control through a
modern web UI and a documented REST API.

Feature target: **parity with the "Graylog Open" column** of the Graylog
feature list, without the JVM, Elasticsearch or MongoDB. Tollan is one static
binary plus a `data/` directory.

- **Zero external dependencies**: pure Go, `CGO_ENABLED=0`. Log storage is
  SQLite (one file per UTC day, FTS5-indexed); metadata is a second SQLite file.
- **Runs three ways**: Docker (scratch, multi-arch), a native Linux binary that
  self-registers as a **systemd** service, and a native Windows binary that
  self-registers as a **Windows Service**.
- **Companion `tollan-agent`** for fleet log collection with centralized config.

Licensed under Apache-2.0.

> **Release status:** the release and Docker publishing workflows exist, but no
> GitHub release, git tag or container image has been published yet. Until the
> first release, build Tollan from source or build the Docker image locally (see
> [Installation](#installation)).

---

## Contents

- [Screenshots](#screenshots)
- [Features](#features)
- [Requirements](#requirements)
- [Installation](#installation)
- [Inputs](#inputs)
- [Search query language](#search-query-language)
- [Streams](#streams)
- [Pipeline rules](#pipeline-rules)
- [Alerts and notifications](#alerts-and-notifications)
- [Outputs](#outputs)
- [The agent](#the-agent)
- [Configuration](#configuration)
- [Users, roles and API tokens](#users-roles-and-api-tokens)
- [REST API](#rest-api)
- [Metrics](#metrics)
- [Architecture](#architecture)
- [Security notes](#security-notes)
- [Troubleshooting](#troubleshooting)
- [Development](#development)
- [Contributing](#contributing)
- [License](#license)

---

## Screenshots

### Search
Lucene-style query language, a time histogram, and a guided-search sidebar of
fields with click-to-filter values.

![Search](https://raw.githubusercontent.com/t0mer/Tollan/main/assets/screenshots/search.png)

### Dashboards
A responsive widget grid (stat tiles, bar/line/area/pie charts, histograms,
top-N tables and a GeoIP world map) with per-widget queries, auto-refresh, a
full-screen TV mode, and per-widget CSV export.

![Dashboard](https://raw.githubusercontent.com/t0mer/Tollan/main/assets/screenshots/dashboard.png)

### Streams & pipelines
Route messages into categories with match rules; normalize and enrich them with
`when … then …` rule pipelines (grok, key=value, JSON/CSV, GeoIP, lookups).

![Streams](https://raw.githubusercontent.com/t0mer/Tollan/main/assets/screenshots/streams.png)

### Alerts
Filter and aggregation event definitions that fan out to Shoutrrr (Slack,
Discord, Telegram, email, generic webhooks and more), Green-API (WhatsApp cloud)
and self-hosted WhatsApp Web channels. Channel credentials are encrypted at rest.

![Alerts](https://raw.githubusercontent.com/t0mer/Tollan/main/assets/screenshots/alerts.png)

### Fleet
`tollan-agent` collectors with status, shipped volume and centrally managed
collector configuration.

![Fleet](https://raw.githubusercontent.com/t0mer/Tollan/main/assets/screenshots/fleet.png)

### Overview
Server health and build at a glance.

![Overview](https://raw.githubusercontent.com/t0mer/Tollan/main/assets/screenshots/overview.png)

The UI is mobile-first and ships a system-aware light/dark theme.

---

## Features

- **Inputs**: syslog (RFC 3164/5424), GELF, CEF, Beats (Lumberjack v1/v2),
  HTTP-JSON (single object or NDJSON bulk), raw text, NetFlow v5/v9 and IPFIX,
  and Docker container logs via the Engine API. TCP inputs support TLS.
- **Disk journal**: an append-only, size-bounded, segmented ingest queue that
  absorbs bursts before processing.
- **Log store**: one SQLite database per UTC day with an FTS5 full-text index.
- **Search**: a Lucene-style query language, time histogram, field facets,
  aggregations (count, sum, avg, min, max, p50/p90/p95/p99), saved searches, and
  CSV/JSON export.
- **Streams**: route messages by match rules, with optional per-stream retention.
- **Pipelines**: `when`/`then` rules with grok, regex, key=value, JSON and CSV
  extraction, type coercion, CSV lookup tables, GeoIP enrichment, routing and
  dropping.
- **Dashboards**: table, histogram, bar, line, area, pie, stat, top-N and map
  widgets.
- **Alerts**: filter and aggregation event definitions, evaluated every 30
  seconds, with custom message templates and a grace period.
- **Notifications**: Shoutrrr, Green-API and WhatsApp Web channels, with
  secrets encrypted with AES-256-GCM.
- **Outputs**: forward messages over GELF (TCP/UDP), raw TCP, TCP syslog
  (RFC 5424) or to stdout, optionally filtered by stream.
- **Retention**: global day-partition retention plus shorter per-stream
  retention, swept nightly.
- **Access control**: local users (argon2id), Admin/Editor/Viewer roles,
  session cookies and per-user API tokens.
- **Content packs**: export and import streams, pipelines, lookups, dashboards,
  event definitions, outputs and saved searches as one JSON bundle.
- **Fleet**: `tollan-agent` enrollment, heartbeats and server-pushed collector
  config.
- **Operations**: Prometheus `/metrics`, `/health`, OS service install
  (systemd / Windows SCM), structured logging (text or JSON).

---

## Requirements

- **Running**: nothing beyond the binary (Linux, macOS or Windows) or Docker.
  Disk space for the `data/` directory (journal up to 1 GiB by default, plus
  log partitions).
- **Optional**: a MaxMind- or IPinfo-format `.mmdb` database for GeoIP
  enrichment. None is bundled (licensing); download one yourself, for example
  MaxMind GeoLite2 City (free account and license terms apply) or an IPinfo
  mmdb.
- **Building**: Go 1.25 (see `go.mod`) and Node 20 for the web UI.

---

## Installation

### Docker (build locally)

No image is published yet, so build it from the repository. The `Dockerfile`
builds the UI, cross-compiles the server and produces a `scratch` image that
runs as uid `65534` with its data in `/data`:

```bash
docker build -t techblog/tollan:local .

docker run -d --name tollan \
  -p 8080:8080 \
  -p 514:1514/udp -p 514:1514/tcp \
  -p 12201:12201/udp -p 12201:12201/tcp \
  -p 5044:5044 -p 2055:2055/udp -p 4739:4739/udp \
  -v tollan-data:/data \
  techblog/tollan:local
```

Or with the provided `docker-compose.yml`: it references `techblog/tollan:latest`,
so uncomment `build: .` (and remove or override `image:`) until the image is
published, then:

```bash
docker compose up -d --build
```

Open <http://localhost:8080> and complete the first-run admin setup.

When published, the Docker workflow pushes `techblog/tollan:latest` and
`techblog/tollan:<YYYY.M.PATCH>` to Docker Hub for `linux/amd64`, `linux/arm64`
and `linux/arm/v7`. A separate manual workflow can push the same platforms to
`ghcr.io/t0mer/tollan`.

### Binary (build from source)

```bash
git clone https://github.com/t0mer/Tollan.git
cd Tollan
(cd web && npm ci && npm run build)   # embed the UI
go build ./cmd/tollan
go build ./cmd/tollan-agent
```

Then:

```bash
./tollan run                                  # foreground, data in ./data
sudo ./tollan service install                 # register as a systemd / Windows service
sudo ./tollan service start
sudo ./tollan admin create --data-dir /var/lib/tollan   # optional: create an admin from the CLI
```

`tollan service` also supports `stop`, `restart`, `uninstall` and `status`.
Service installs use a per-OS data directory unless you pass `--data-dir`:
`/var/lib/tollan` on Linux and `C:\ProgramData\Tollan` on Windows. The resolved
`--data-dir`, `--log-level`, `--log-format`, `--http-addr`, `--auth` and (as an
absolute path) `--config` are baked into the service definition. Under a service
manager, Tollan also appends its log to `tollan.log` in the data directory.

On Linux, run `service install` as root so it installs hardened: it creates a
dedicated `tollan` system user, `chown`s the data directory, and writes a unit
with `Restart=on-failure`, `LimitNOFILE=65536` and
`AmbientCapabilities=CAP_NET_BIND_SERVICE`, so inputs may bind privileged ports
such as 514. Without root it prints what to create manually.

`admin create` writes directly to the metadata database, so point it at the same
`--data-dir` the server uses. You can also create the first admin in the web UI.

When releases are published, the Release workflow builds archives with
GoReleaser: `tollan_<version>_<os>_<arch>.tar.gz` (`.zip` on Windows), each
containing both binaries, `README.md`, `LICENSE` and `config.example.yaml`, for
Linux amd64/arm64/armv6/armv7/386, macOS amd64/arm64 and Windows amd64/arm64.

---

## Inputs

Inputs are declared in the YAML config file under `inputs:` and started when the
server starts; changing them requires a restart. The **Inputs** page and
`GET /api/v1/inputs` list them read-only.

If `inputs:` is omitted, Tollan starts this default set on unprivileged ports:

| ID | Type | Bind | Protocol |
|---|---|---|---|
| `syslog-udp` | `syslog` | `:1514` | UDP |
| `syslog-tcp` | `syslog` | `:1514` | TCP |
| `gelf-udp` | `gelf` | `:12201` | UDP |
| `gelf-tcp` | `gelf` | `:12201` | TCP |
| `beats` | `beats` | `:5044` | TCP |
| `netflow` | `netflow` | `:2055` | UDP |
| `ipfix` | `ipfix` | `:4739` | UDP |

Declaring any input replaces the whole default set, so list every input you
want. Map host port 514 to 1514 in Docker for standard syslog.

| Type | Protocols (`protocol:`) | Framing / notes |
|---|---|---|
| `syslog` | `udp`, `tcp`, `tls` | RFC 3164 and 5424 autodetected, structured data kept as `sd.*` fields. TCP autodetects octet-counting or newline framing (RFC 6587). |
| `gelf` | `udp`, `tcp`, `tls` | GELF 1.1. UDP supports chunking and gzip/zlib; TCP is null-byte delimited. |
| `cef` | `udp`, `tcp`, `tls` | ArcSight CEF, one message per datagram or per line. |
| `beats` | TCP (`tls` for TLS) | Lumberjack v1 and v2 (Filebeat, Winlogbeat, …). |
| `httpjson` | HTTP | `POST` to any path: one JSON object, or NDJSON for bulk. Optional `token`, sent as the `X-API-Token` header. Body up to 16 MiB. No TLS. |
| `raw` | `udp`, `tcp`, `tls` | Plain text, one message per datagram or per line (max 1 MiB). |
| `netflow` / `ipfix` | UDP | NetFlow v5, v9 and IPFIX (both types decode all three). |
| `docker` | Engine API | `bind` is `unix:///…`, `tcp://…` or `http(s)://…`; defaults to `unix:///var/run/docker.sock`. |

Every input takes `id`, `type` and `bind`. `protocol` is required for the
`syslog`, `gelf`, `cef` and `raw` types. For `protocol: tls` (and TLS on
`beats`) set `tls_cert_file` and `tls_key_file`; TLS 1.2 is the minimum.

```yaml
inputs:
  - {id: syslog-udp, type: syslog,   bind: ":1514",  protocol: udp}
  - {id: syslog-tls, type: syslog,   bind: ":6514",  protocol: tls,
     tls_cert_file: /data/tls/cert.pem, tls_key_file: /data/tls/key.pem}
  - {id: gelf-tcp,   type: gelf,     bind: ":12201", protocol: tcp}
  - {id: cef-udp,    type: cef,      bind: ":5514",  protocol: udp}
  - {id: http-json,  type: httpjson, bind: ":8888",  token: "change-me"}
  - {id: docker,     type: docker,   bind: "unix:///var/run/docker.sock"}
```

The Dockerfile and `docker-compose.yml` expose 6514/tcp for syslog over TLS, but
no TLS input is started by default: add one as above.

HTTP-JSON example:

```bash
curl -X POST http://tollan-host:8888/ \
  -H 'X-API-Token: change-me' \
  -d '{"message":"user login","host":"web-01","user":"alice","level":"info"}'
```

For JSON-based inputs, the body is taken from `message`, `short_message`, `msg`
or `log`; the source from `host`, `hostname`, `source` or `host.name`; and the
timestamp from `timestamp`, `@timestamp` or `time`. Nested objects are flattened
into dotted field names.

### Streaming Docker container logs

Add a `docker` input and give Tollan access to the Docker socket. It
auto-discovers running and newly started containers, follows their stdout/stderr,
and enriches each line with `container_name`, `image` and `container_stream`:

```yaml
inputs:
  - { id: docker, type: docker, bind: "unix:///var/run/docker.sock" }
```

In Docker, mount the socket (`/var/run/docker.sock:/var/run/docker.sock`) and
pass a config with the input; see `docker-compose.yml`. A `:ro` mount only stops
the socket file from being replaced; it does **not** make the Docker API
read-only. The image runs as uid `65534`, which needs read/write access to the
socket, for example via `group_add` with the socket's group ID. Socket access is
full, root-equivalent control of the Docker host.

The socket input only sees containers on **Tollan's own host**. To collect
containers running on **other servers**, ship them over the network with
Docker's built-in log driver (per container or per Compose service); see below.

### Shipping container logs from remote servers (per container / Compose)

Docker's built-in **GELF** log driver sends straight to Tollan's GELF input over
the network, with no agent and no Tollan-side config. Configure it per container:

```bash
docker run \
  --log-driver=gelf \
  --log-opt gelf-address=tcp://tollan-host:12201 \
  --log-opt tag="{{.Name}}" \
  nginx
```

Or per service in **docker-compose.yml** on each server:

```yaml
services:
  api:
    image: my/api
    logging:
      driver: gelf
      options:
        gelf-address: "tcp://tollan-host:12201"
        tag: "api"
        labels: "env,team"       # forward these container labels as fields
    labels:
      env: production
      team: payments
```

Each event arrives with `container_name`, `image`, `command`, `tag` and any
forwarded `labels` as searchable fields. The GELF `host` field is set to the
**origin server's hostname**, so logs from different servers are distinguished by
`source:`, for example `source:web-01 AND container_name:api`. Add an explicit
tag (`--log-opt tag="{{.Name}}@dc1"`) if you want a custom origin label.

Use `tcp://` for reliable delivery of large log lines; `udp://` is fire-and-forget
(Tollan reassembles chunked GELF UDP). The **syslog** driver
(`--log-driver=syslog --log-opt syslog-address=udp://tollan-host:1514`) works the
same way against Tollan's syslog input.

To avoid reconfiguring every container on a host, run one **`tollan-agent`** (or a
Tollan `docker` input with the mounted socket) per server instead.

---

## Search query language

A Lucene-style subset, compiled to SQL over typed columns and the FTS5 index:

```
level:error AND source:web01           field predicates + boolean operators
level:error source:web01               juxtaposition is an implicit AND
status:>=500                           numeric comparison (> < >= <=)
status:[400 TO 599]                    inclusive range
status:{400 TO 500}                    exclusive range ([a TO b} mixes both)
source:web*   host:db0?                wildcards: * and ?
_exists_:src_ip                        field presence
"disk full" OR timeout                 phrases and free text
message:timeout                        message/body search the full-text index
NOT level:debug
(level:error OR level:critical) AND NOT source:lb01
```

`source`, `stream`, `input_id` (or `input`) and `id` map to dedicated columns; all
other fields are looked up in the message's JSON fields. Syslog, GELF and CEF
inputs set `level` to one of `emergency`, `alert`, `critical`, `error`,
`warning`, `notice`, `info`, `debug`.

Time ranges accept `now`, `now-<duration>` using Go duration units (`now-15m`,
`now-24h`, `now-168h`; days are not a unit) or RFC 3339 timestamps. Search
supports a result histogram, field facets, aggregations, saved searches, and
export (CSV or JSON, up to 100,000 rows).

---

## Streams

A stream is a named category with match rules, combined with `and` or `or`.
Each rule has a `field`, a `type` and a `value`, and can be negated:

| Rule type | Matches when |
|---|---|
| `exact` | the field equals the value |
| `contains` | the field contains the value |
| `regex` | the field matches the regular expression |
| `presence` | the field exists |
| `gt` / `lt` | the numeric field is greater / less than the value |

Each message goes to **one** stream: the first matching stream wins, unmatched
messages go to the built-in `default` stream ("All messages"), and a pipeline
`route()` action overrides matching. A stream can set `retention_days`
shorter than the global retention.

---

## Pipeline rules

Pipelines are ordered rules attached to stages: `_all` runs on every message
before routing; a stream ID runs after a message is routed to that stream. Each
rule has a `when` condition and a `then` list with **one action per line**:

```
when:  has(src_ip) && cidr(src_ip, "10.0.0.0/8")
then:  set("network", "internal")
       geoip(src_ip)

when:  eq(program, "nginx")
then:  grok(message, "%{IPORHOST:clientip} .* \"%{HTTPMETHOD:verb} %{URIPATH:path}")
       coerce(status, "int")
```

Field names can be bare identifiers (dots and dashes allowed) or quoted strings.
An empty condition or `true` always matches.

**Conditions**, combined with `&&`, `||`, `!` and parentheses:

| Function | Meaning |
|---|---|
| `has(field)` | field is present |
| `eq(field, value)` / `neq(field, value)` | string equality / inequality |
| `contains(field, substring)` | substring match |
| `regex(field, pattern)` | regular-expression match |
| `cidr(field, "10.0.0.0/8")` | IP address is inside the network |
| `gt` / `lt` / `gte` / `lte(field, number)` | numeric comparison |

**Actions:**

| Action | Effect |
|---|---|
| `set(field, value)` | set a field |
| `rename(from, to)` | rename a field |
| `remove(field)` | delete a field |
| `coerce(field, type)` | convert to `int`, `float`, `bool` or `string` |
| `parse_json(field)` | parse a JSON object into fields |
| `parse_kv(field)` | parse `key=value` pairs into fields |
| `parse_csv(field, "col1,col2,…")` | parse CSV into the named columns |
| `grok(field, pattern)` | grok-extract named captures |
| `regex_extract(field, pattern)` | extract named regex groups |
| `lookup(table, key_field, target_field)` | set `target_field` from a lookup table |
| `geoip(field)` | add `<field>_geo_country`, `_country_name`, `_city`, `_location` (`lat,lon`) |
| `route(stream_id)` | send the message to a stream |
| `drop()` | discard the message |

Built-in grok patterns: `INT`, `NUMBER`, `WORD`, `NOTSPACE`, `SPACE`, `DATA`,
`GREEDYDATA`, `QUOTEDSTRING`, `IPV4`, `IP`, `HOSTNAME`, `IPORHOST`, `USER`,
`USERNAME`, `EMAILADDRESS`, `POSINT`, `URIPATH`, `URIPARAM`, `URIPATHPARAM`,
`HTTPMETHOD`, `LOGLEVEL`, `MONTH`, `MONTHDAY`, `TIME`, `SYSLOGTIMESTAMP`,
`TIMESTAMP_ISO8601`, `COMMONAPACHELOG`, `SSHDFAIL`, `IPTABLES`, `SOPHOSHEADER`.

**Lookup tables** are CSV files with a header row, loaded from a local file
(`source_type: file`) or an HTTP(S) URL (`source_type: url`), keyed by
`key_column` → `value_column`. They are managed through the API
(`/api/v1/lookups`) and reloaded at startup and whenever configuration changes.

**GeoIP** uses a MaxMind- or IPinfo-format `.mmdb` you supply (`geoip.db_path`);
no database is bundled. Without one, `geoip()` is a no-op.

---

## Alerts and notifications

Event definitions are evaluated every 30 seconds over a sliding window
(`window_seconds`, default 300):

- **Filter**: fires when the number of messages matching `query` in the window
  is at least `threshold`. The notification includes up to `backlog` sample
  messages (default 5).
- **Aggregation**: groups matches by `group_by`, computes `metric` (`count`,
  `sum`, `avg`, `min`, `max`, `p50`, `p90`, `p95`, `p99`) over `metric_field`,
  and fires once per group whose value is above `threshold`.

`grace_seconds` suppresses repeat firings, and `message_template` is a Go
template with `{{.Definition}}`, `{{.Count}}`, `{{.Threshold}}`,
`{{.WindowSeconds}}`, `{{.GroupKey}}` and `{{.Samples}}`. Firings are stored and
shown on the **Alerts** page (`GET /api/v1/events`).

Notification channels (`/api/v1/notifications`):

| Provider | Fields |
|---|---|
| `shoutrrr` | `url`: any Shoutrrr URL, e.g. `slack://…`, `discord://…`, `telegram://…`, `smtp://…`, `generic://…` (webhook) |
| `greenapi` (Green-API, WhatsApp cloud) | `instance_id`, `token`, `phone` (digits only; `@c.us` is appended), optional `api_url` (default `https://api.green-api.com`) |
| `whatsapp_web` (self-hosted gateway) | `base_url`, `phone`, optional `username` / `password` (Basic Auth); posts to `<base_url>/send/message` |

Channels can be enabled or disabled, and **Send test** delivers a real message
before saving. `url`, `token` and `password` are encrypted at rest and returned
masked (`***`) by the API.

---

## Outputs

Outputs forward processed messages to other systems (`/api/v1/outputs`, or the
**Outputs** page under System):

| Type | Target |
|---|---|
| `gelf` | `address` (`host:port`) over `protocol` `tcp` or `udp` |
| `tcp_raw` | one message per line over TCP |
| `tcp_syslog` | RFC 5424 over TCP |
| `stdout` | the server's standard output |

Set `stream` to forward a single stream; leave it empty for all. Each output has
a buffered worker with reconnect and retry; when the buffer is full, messages are
dropped and counted in `tollan_output_failures_total`. Outputs do not use TLS.

---

## The agent

`tollan-agent` tails files (and journald on Linux), ships each line to the
server as GELF over TCP, sends heartbeats every 30 seconds, and applies
server-pushed collector config.

```bash
tollan-agent run \
  --server http://tollan:8080 \
  --token  "<enrollment-token>" \
  --file '/var/log/*.log' --tags web,prod

sudo tollan-agent service install --server http://tollan:8080 --token … --file '/var/log/*.log'
```

| Flag | Default | Purpose |
|---|---|---|
| `--server` | (required) | Tollan server base URL |
| `--token` | — | Enrollment token (`agent.enrollment_token` on the server) |
| `--gelf-addr` | server host `:12201` | GELF **TCP** target (the server needs a GELF TCP input) |
| `--file` | — | File glob to tail (repeatable) |
| `--tags` | — | Comma-separated tags |
| `--data-dir` | `./agent-data` | State directory (agent ID and secret) |
| `--log-level` | `info` | `debug` / `info` / `warning` / `error` |

Subcommands: `run`, `service install|uninstall|start|stop|restart`, `version`.

On first start the agent generates an ID, enrolls with
`POST /api/v1/agents/register`, and receives a per-agent secret (stored hashed
on the server) that authenticates its heartbeat and config calls. It then shows
up on the **Fleet** page, where its tags and collector config (file globs,
journald) can be edited centrally. Files are tailed from their current end.
`--file` globs from the command line are expanded only once, at startup, so files
created later need an agent restart. Server-pushed globs are re-applied after
every successful heartbeat, so new matching files are picked up within about 30
seconds. The config schema also has `multiline_pattern` and
`windows_event_log` fields, which the current agent does not act on.

On Linux the agent service runs as the `tollan` user, which the agent does not
create: install the server service first, or create the user yourself.

---

## Configuration

Precedence: **flags > environment (`TOLLAN_` prefix) > YAML file > defaults.**
Pass a YAML file with `--config`; see `config.example.yaml`.

| YAML key | Flag | Env | Default | Purpose |
|---|---|---|---|---|
| — | `--config` | — | — | Path to a YAML config file |
| `data_dir` | `--data-dir` | `TOLLAN_DATA_DIR` | `./data` | Journal, log partitions, metadata, secret key |
| `log.level` | `--log-level` | `TOLLAN_LOG_LEVEL` | `info` | `debug` / `info` / `warning` / `error` |
| `log.format` | `--log-format` | `TOLLAN_LOG_FORMAT` | `text` | `text` or `json` |
| `http.addr` | `--http-addr` | `TOLLAN_HTTP_ADDR` | `:8080` | UI/API listen address |
| `http.read_timeout` | — | `TOLLAN_HTTP_READ_TIMEOUT` | `30s` | HTTP read timeout |
| `http.write_timeout` | — | `TOLLAN_HTTP_WRITE_TIMEOUT` | `60s` | HTTP write timeout |
| `http.idle_timeout` | — | `TOLLAN_HTTP_IDLE_TIMEOUT` | `120s` | HTTP idle timeout |
| `auth.mode` | `--auth` | `TOLLAN_AUTH_MODE` | `enabled` | `enabled` or `disabled` (open lab mode) |
| `retention.days` | — | `TOLLAN_RETENTION_DAYS` | `90` | Global retention in days; `0` disables |
| `journal.max_segment_bytes` | — | `TOLLAN_JOURNAL_MAX_SEGMENT_BYTES`\* | 16 MiB | Journal segment size |
| `journal.max_total_bytes` | — | `TOLLAN_JOURNAL_MAX_TOTAL_BYTES`\* | 1 GiB | Journal size cap |
| `geoip.db_path` | — | `TOLLAN_GEOIP_DB_PATH`\* | — | Path to a `.mmdb` file |
| `agent.enrollment_token` | — | `TOLLAN_AGENT_ENROLLMENT_TOKEN`\* | — (open) | Token required for agent enrollment |
| `inputs` | — | — | default set | See [Inputs](#inputs) |

\* Keys without a built-in default can only be set from the environment when
the key also appears in the YAML file (Viper only applies environment overrides
to keys it already knows). For example, `TOLLAN_GEOIP_DB_PATH` works once
`geoip.db_path` is present in the YAML file, even with an empty value. `inputs`
must come from the YAML file.

The data directory holds `journal/`, `logs/<YYYY-MM-DD>.db`, `tollan.db`
(metadata: users, tokens, streams, pipelines, dashboards, events, channels,
agents) and `secret.key`. Back up the whole directory; without `secret.key`,
stored notification secrets cannot be decrypted and sessions are invalidated.

**Retention** runs once at startup and nightly at 03:00: it deletes whole day
partitions older than `retention.days`, and prunes rows from streams whose
`retention_days` is shorter than the global value.

**Content packs** (**System** page, `/api/v1/content-packs`) export streams,
pipelines, lookups, dashboards, event definitions, outputs and saved searches as
one JSON file. Notification channels are excluded because they hold secrets.
Import supports `?dry_run=true` to preview creates and updates.

---

## Users, roles and API tokens

With `auth.mode: enabled`, the API and UI stay **open until the first admin
exists**; create it in the first-run wizard or with `tollan admin create`.

| Role | Can |
|---|---|
| **Admin** | Everything, including user management |
| **Editor** | Read everything, and create/change/delete configuration |
| **Viewer** | Read-only |

Any signed-in user can manage their own API tokens. Passwords are hashed with
argon2id. Browser sessions are HMAC-signed cookies (`tollan_session`) valid for
12 hours, marked `Secure` when the request arrives over HTTPS (directly or with
`X-Forwarded-Proto: https`). API tokens (`tol_…`) are shown once, stored as
SHA-256 hashes, and sent as `Authorization: Bearer <token>`.

With `auth.mode: disabled` everyone is treated as an admin and the UI shows a
warning banner.

---

## REST API

Everything the UI does is available through the REST API under `/api/v1`. The
OpenAPI 3 specification is served at `/api/openapi.yaml`; `/api/docs` is a
simple landing page that links to it. The canonical spec lives in
[`api/openapi.yaml`](api/openapi.yaml).

| Area | Endpoints |
|---|---|
| Auth | `GET /auth/status`, `POST /auth/setup`, `POST /auth/login`, `POST /auth/logout`, `GET /auth/me` |
| Users (admin) | `GET/POST /users`, `PUT/DELETE /users/{id}` |
| API tokens | `GET/POST /tokens`, `DELETE /tokens/{id}` |
| Search | `GET /search`, `/search/histogram`, `/search/fields`, `/search/aggregate`, `/search/export` |
| Saved searches | `GET/POST /saved-searches`, `PUT/DELETE /saved-searches/{id}` |
| Config entities | `GET/POST` and `PUT/DELETE /{id}` on `/streams`, `/pipelines`, `/lookups`, `/dashboards`, `/event-definitions`, `/outputs` |
| Notifications | `GET/POST /notifications`, `POST /notifications/test`, `PUT/DELETE /notifications/{id}` |
| Events | `GET /events` |
| Inputs | `GET /inputs` |
| Content packs | `GET /content-packs/export`, `POST /content-packs/import` |
| Fleet | `GET /agents`, `PUT/DELETE /agents/{id}`, `POST /agents/register`, `POST /agents/{id}/heartbeat`, `GET /agents/{id}/config` |
| Build | `GET /version` |

Search endpoints take `q`, `from`, `to` and `stream`; `/search` also takes
`limit` (max 1000), `offset` and `order` (`asc`/`desc`), and `/search/export`
takes `format=csv|json` and `limit`. For example:

```bash
curl -H "Authorization: Bearer $TOLLAN_TOKEN" \
  "http://localhost:8080/api/v1/search?q=level:error&from=now-1h&limit=20"
```

`GET /health` returns `{"status":"ok","version":"…"}`.

---

## Metrics

Prometheus metrics are exposed at `/metrics` (no authentication), alongside the
Go runtime and process collectors:

| Metric | Labels | Meaning |
|---|---|---|
| `tollan_build_info` | `version`, `commit`, `goversion` | Build information |
| `tollan_ingest_messages_in_total` | `input_id`, `type` | Messages received |
| `tollan_output_messages_out_total` | `output_id`, `type` | Messages forwarded |
| `tollan_output_failures_total` | `output_id` | Output drops/failures and notification send failures |
| `tollan_journal_depth_messages` | — | Unprocessed journal messages |
| `tollan_journal_utilization_ratio` | — | Journal size as a fraction of the cap |
| `tollan_processing_lag_seconds` | — | Age of the oldest unprocessed message |
| `tollan_event_firings_total` | `definition_id` | Event firings |
| `tollan_store_day_size_bytes` | `day` | Registered, not yet populated |
| `tollan_search_latency_seconds` | — | Registered, not yet populated |

---

## Architecture

```
inputs → disk journal → processing     → stream router → SQLite
         (bounded,       (decode, pipelines,    │             day store
          append-only)    extractors, GeoIP,    │             (FTS5)
                          lookups)              ├→ outputs (GELF / TCP / syslog / stdout)
                                                └  events (every 30 s) → notifications
```

Inputs only frame messages and append them to the **journal**, then return. A
single processing consumer decodes each message by input type, runs the
pipelines, routes it and writes it in batches (up to 2,000 messages) to the day
store, resuming from its committed position after a restart (at-least-once).
The journal is an append-only, segmented queue bounded by size: ingest bursts
are absorbed on disk and processed asynchronously, and under sustained overload
the **oldest unprocessed data is dropped** when the cap is reached.

On the reference x86 box, ingest sustains **>150k msg/s** into the journal,
which drains through decode + FTS indexing at **~8k msg/s**.
<!-- TODO: verify benchmark figures (no benchmark in the repository) -->

---

## Security notes

- **Create the admin immediately.** Until the first user exists, the whole API
  is open. Never expose `auth.mode: disabled` beyond a lab.
- **Use TLS in front of the UI/API.** Tollan serves plain HTTP; put it behind a
  reverse proxy that terminates TLS and sets `X-Forwarded-Proto: https` so
  session cookies are marked `Secure`.
- **Limit exposure of ingest ports.** Most inputs accept data from anyone who can
  reach them. Firewall them to known senders, set a `token` on HTTP-JSON inputs,
  use `tls` inputs across untrusted networks, and set `agent.enrollment_token`
  (enrollment is open without it).
- **`/metrics` and `/health` are unauthenticated**; restrict them at the network
  or proxy level if needed.
- **Protect the data directory.** `secret.key` encrypts notification credentials
  (AES-256-GCM) and signs session cookies; `tollan.db` holds password and token
  hashes. Anyone with the directory can read your logs.
- **Mount the Docker socket deliberately.** Socket access is full,
  root-equivalent control of the Docker host, even with a `:ro` mount (that
  only protects the socket file, not the API).
- **Logs contain personal data.** Set retention (`retention.days` and
  per-stream `retention_days`) to match your policy. There are no per-stream
  permissions: every role can search every message, so limit who gets an
  account, and use retention or `drop()` pipeline rules for sensitive data.
- **Editors can change configuration that reaches out**: lookup tables read
  local files or fetch URLs from the server, and notifications and outputs send
  data to arbitrary endpoints. Grant the Editor role only to trusted users.

---

## Troubleshooting

- **Syslog on port 514 doesn't bind.** Ports below 1024 need privileges. Use the
  default 1514 and map 514→1514 in Docker, or run as the systemd service (which
  has `CAP_NET_BIND_SERVICE`).
- **Configured one input and the others stopped.** Declaring `inputs:` replaces
  the default set; list all inputs you need.
- **Docker input logs nothing.** The container runs as uid 65534 and needs
  read/write access to the mounted socket (e.g. `group_add` with the socket's
  group ID). If the socket was never reachable, the error is logged only at
  debug level: run with `--log-level debug` or `TOLLAN_LOG_LEVEL=debug`.
- **Agent enrolls but no logs arrive.** The agent ships GELF over TCP to
  `<server host>:12201` by default. Make sure a GELF TCP input exists and is
  reachable, or set `--gelf-addr`.
- **`now-7d` is rejected.** Relative times use Go durations; use `now-168h`.
- **GeoIP fields are missing.** Set `geoip.db_path` in the YAML file and
  restart. `TOLLAN_GEOIP_DB_PATH` only takes effect when the key also appears in
  the YAML file.
- **`tollan admin create` user can't log in.** Make sure you used the same
  `--data-dir` as the server (`/var/lib/tollan` for Linux service installs).

---

## Development

Requires Go (see `go.mod`) and Node 20 for the UI.

```bash
cd web && npm ci && npm run build   # builds the embedded UI into web/dist
go build ./cmd/tollan               # server
go build ./cmd/tollan-agent         # agent
go vet ./... && go test ./...       # checks run by CI
```

- `scripts/dev.sh` runs the backend (`:8080`, debug logging, `./data`) and the
  Vite dev server (`:5173`, proxying `/api`, `/health` and `/metrics`) together.
- `scripts/build.sh` cross-compiles both binaries into `dist/` for Linux
  amd64/arm64/armv7/armv6/386, macOS amd64/arm64 and Windows amd64/arm64
  (`REBUILD_UI=1` forces a UI rebuild).
- `scripts/next-version.sh` prints the next `YYYY.M.PATCH` version from git tags.

Project layout:

```
cmd/tollan/          server CLI (run, service, admin, version)
cmd/tollan-agent/    agent CLI
internal/            api, auth, config, crypto, decode, event, geoip, input,
                     journal, logstore, lookup, meta, metrics, notify, output,
                     pipeline, processing, retention, search, server, stream, svc
api/openapi.yaml     REST API spec (embedded)
web/                 React + TypeScript + Vite UI (embedded from web/dist)
```

GitHub Actions workflows:

| Workflow | Trigger | Does |
|---|---|---|
| `ci.yml` | push to `main`, pull requests | `go vet`, `go build`, `go test`; UI `npm ci` + build |
| `release.yml` | manual | Tags `YYYY.M.PATCH` and publishes a GitHub release with GoReleaser |
| `docker-image.yml` | manual, or after a successful Release | Pushes `techblog/tollan` to Docker Hub (amd64, arm64, arm/v7) |
| `publish-ghcr.yml` | manual | Pushes `ghcr.io/t0mer/tollan` (amd64, arm64, arm/v7) |

---

## Contributing

Issues and pull requests are welcome. Please run `go vet ./...`, `go test ./...`
and the UI build before opening a pull request, and keep API changes in sync
with `api/openapi.yaml`.

---

## License

Apache-2.0. See [LICENSE](LICENSE).
