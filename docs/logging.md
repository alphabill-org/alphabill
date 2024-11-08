# Logging in Alphabill

This document describes structured logging rules for Alphabill.

The [stdlib slog](https://pkg.go.dev/log/slog) is used as a logger and
"structured logging" is achieved by adding additional 
[attributes](https://pkg.go.dev/log/slog#Attr) to log calls, ie

```go
logger.InfoContext(ctx, "meaning of everything", slog.Int("answer", 42))
```
The names of attributes (ie known `Attr.Key` values) is the Alphabill
logging schema.

## Schema

This schema is defined from the logger POV ie what attribute names are in use
when log calls in the code are made. The name in the final log output might
differ as it depends on the log handler!

Use attributes judiciously, not everything has to be "structured".
It makes sense to add attribute if there is need to search / filter logs
by that information.
For example currently we do not add partition id to the logs as
attributes as this info is deducible from different fields in Kibana (ie
`host.name`, `nomad.task.name` etc) and in the local env it is easy either
to log to different files or consoles.

Before adding new attribute see 
[ECS Field Reference](https://www.elastic.co/guide/en/ecs/current/ecs-field-reference.html)
is there already suitable field defined.
Consult DevOps/infra will there be a need to add rules into log ingest pipeline
to transform the attribute.
Attribute names should be in snake_case.

### Attributes defined by the `slog`

Following attributes are "created by logger":

| name | type | comment |
|---|---|---|
| msg | string | message to be logged |
| time | time.Time | timestamp when the log call was made |
| level | string | name of the log level |
| source | struct | information about the [location in the source](https://pkg.go.dev/log/slog#Source) code from where the log call was made. |

### Attributes defined by AB logger

AB logger package defines some "well known attributes".
For these there is a "constructor function" in the logger package to create
the attribute - use that rather than creating slog.Attr manually!

| name | type | comment |
|---|---|---|
| node_id | string | libp2p peer ID is used as node id - which node is logging. |
| go_id | int | goroutine id, added by the AB logger handler if enabled |
| round | int | current round number (depends on the context whether it was a root chain round or validator round!) |
| err | error | error which caused the log message to be created (log level doesn't have to be ERROR). |
| unit_id | []byte | hex encoded ID of the unit (bill, token, token type, ...) which caused the log record |
| data | any | meant to attach some data struct to the message, ie tx |
| module | string | reserved for future use |

## Output schema

The logger's "output schema" depends on the format configuration (slog handler).

### ECS

The AB team uses Kibana for logs so the default output handler for AB is `ECS`
which is 
[Elastic Common Schema](https://www.elastic.co/guide/en/ecs/current/ecs-field-reference.html)
"friendly" format. Ie some of the well known attributes will be transformed as following

| attribute | ECS | comment |
|---|---|---|
| msg | message | |
| err | error.message | |
| node_id | service.node.name | the `peerIdFormat` configuration is not respected by ECS, it always logs full ID. |
| source.* | log.origin.* | |
| data | data.\<type name\>.* | the \<type name\> is the type name of the data |

## Log levels

 - `ERROR`: unrecoverable fatal errors only - gasp of death - code cannot continue and will terminate.
 - `WARN`: changes in state that affects the service degradation.
 - `INFO`: events that have no effect on service, but can aid in performance, status and statistics monitoring.
 - `DEBUG`: events generated to aid in debugging, application flow and detailed service troubleshooting.

## IDE test runner and logs

By default logger for tests outputs log records with ANSI color codes.
If Your IDE test viewer doesn't support color codes the log lines contain "garbage",
ie something like

> logger.go:225: [2m13:44:38.8725[0m [91mERR[0m [2mlogger/logger_test.go:55[0m now thats really bad [2merr=[0m"what now" [2mgo_id=[0m24

To disable color codes configure IDE so that it executes `go test` tool with
env variable `AB_TEST_LOG_NO_COLORS` set to `true`.

The default log level for test loggers is `debug`. Environment variable `AB_TEST_LOG_LEVEL`
can be used to set the default log level for tests ie

```bash
AB_TEST_LOG_LEVEL=trace go test ./...
```
The `-run` flag and/or specific package name (instead of `./...`) can be used to debug
specific test / package.

#### VSCode

To disable logger emitting color codes in VSCode open `Settings` and search
for `Go: Tools Env Vars`, open the settings json and add the env var there, ie

```json
"go.toolsEnvVars": {
    "AB_TEST_LOG_NO_COLORS":"true"
}
```
