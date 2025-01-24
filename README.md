# Build

Run `make build` to build the application. Executable will be built to `build/alphabill`. 

### Build dependencies

* `golang` version 1.21. (https://go.dev/doc/install)
* `C` compiler, recent versions of [GCC](https://gcc.gnu.org/) are recommended. In Debian and Ubuntu repositories, GCC is part of the build-essential package. On macOS, GCC can be installed with [Homebrew](https://formulae.brew.sh/formula/gcc).

# Money Partition

1. Run script `./setup-testab.sh -m 3 -t 0 -e 0` to generate configuration for a root chain and 3 money partition nodes.
    The script generates root chain and partition node keys, genesis files.
    Node configuration files are located in `testab` directory.
   * Initial bill owner predicate can be specified with flag `-i predicate-in-hex`.
2. Run script `./start.sh -r -p money` to start root chain and 3 money partition nodes
3. Run script `stop.sh -a` to stop the root chain and partition nodes.
   
   Alternatively, use `stop.sh` to stop any partition or root and `start.sh` to resume. See command help for more details. 

# User Token Partition

Typical set-up would run money and user token partition as fee credits need to be added to the user token partition
in order to pay for transactions.
Theoretically it is also possible run only the user token partition on its own, but it would not make much sense.
1. Run script `./setup-testab.sh -m 3 -t 3 -e 0` to generate configuration for a root chain and 3 money and token partition nodes.
   The script generates root chain and partition node keys, genesis files.
   Node configuration files are located in `testab` directory.
2. Run script `./start.sh -r -p money -p tokens` to start root chain and 3 partition nodes (money and token)
3. Run script `stop.sh -a` to stop the root chain and partition nodes.

# EVM Partition

Typical set-up would run money and EVM partition as fee credits need to be added to the EVM partition
in order to create an account and pay for transactions.
Theoretically it is also possible run only the EVM partition on its own, but it would not make much sense.
1. Run script `./setup-testab.sh -m 3 -t 0 -e 3` to generate configuration for a root chain and 3 money and EVM partition nodes.
   The script generates root chain and partition node keys, genesis files.
   Node configuration files are located in `testab` directory.
2. Run script `./start.sh -r -p money -p evm` to start root chain, partition nodes (EVM, money)
3. Run script `stop.sh -a` to stop the root chain and partition nodes.

# Start all partitions at once

1. Run script `./setup-testab.sh` to generate genesis for root, and 3 money, tokens and EVM nodes.
2. Run `start.sh -r -p money -p tokens -p evm` to start everything
3. Run `stop.sh -a` to stop everything

# Configuration

It's possible to define the configuration values from (in the order of precedence):

* Command line flags (e.g. `--address="/ip4/127.0.0.1/tcp/26652"`)
* Environment (Prefix 'AB' must be used. E.g. `AB_ADDRESS="/ip4/127.0.0.1/tcp/26652"`)
* Configuration file (properties file) (E.g. `address="/ip4/127.0.0.1/tcp/26652"`)
* Default values

The default location of configuration file is `$AB_HOME/config.props`

The default `$AB_HOME` is `$HOME/.alphabill`

## Logging configuration

Logging can be configured through a yaml configuration file. See [logger-config.yaml](cli/alphabill/config/logger-config.yaml) for example.

Default location of the logger configuration file is `$AB_HOME/logger-config.yaml`

The location can be changed through `--logger-config` configuration key. If it's relative URL, then it's relative
to `$AB_HOME`. Some logging related parameters can be set via command line parameters too - run `alphabill -h`
for more.

See [logging.md](./docs/logging.md) for information about log schema.

# Distributed Tracing

To enable tracing environment variable `AB_TRACING` (or command line flag `--tracing`) has
to be set to one of the supported exporter names: `stdout`, `otlptracehttp` or `zipkin`.

Exporter can be further configured using
[General SDK Configuration](https://opentelemetry.io/docs/concepts/sdk-configuration/general-sdk-configuration/)
and exporter specific
[OpenTelemetry Protocol Exporter](https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/protocol/exporter.md)
environment variables.
Exceptions are:

- instead of `OTEL_TRACES_EXPORTER` and `OTEL_METRICS_EXPORTER` we use AB specific
  environment variables (ie `AB_TRACING`) or command line flags to select the exporter;
- alphabill sets propagators (`OTEL_PROPAGATORS`)  to “tracecontext,baggage”;
- `OTEL_SERVICE_NAME` is set based on "best guess" of current binary's role (ie to
  "ab.wallet", "ab.tokens", "ab.money",...)

## Tracing tests

To enable trace exporter for test the `AB_TEST_TRACER` environment variable has to be set
to desired exporter name, ie

```sh
AB_TEST_TRACER=otlptracehttp go test ./...
```

The test tracing will pick up the same OTEL environment variables linked above except that
some parameters are already "hardcoded":

- "always_on" sampler is used (`OTEL_TRACES_SAMPLER`);
- the `otlptracehttp` exporter is created with "insecure client transport"
  (`OTEL_EXPORTER_OTLP_INSECURE`);

# Set up autocompletion

To use autocompletion (supported with `bash`, `fish`, `powershell` and `zsh`), run the following commands after
building (this is `bash` example):

* `./alphabill completion bash > /tmp/completion`
* `source /tmp/completion`

# CI setup

See [gitlab-ci.yml](.gitlab-ci.yml) for details.

GitLab runs the CI job inside docker container defined in `alphabill/gitlab-ci-image`.

# Build Docker image with local dependencies

For example, if go.work is defined as follows:

```plain
go 1.23.2

use (
    .
    ../alphabill-go-base
)
```

The folder can add be added into Docker build context by specifying it as follow:

```console
DOCKER_GO_DEPENDENCY=../alphabill-go-base make build-docker
```

## License

This repository contains the source code released under the AGPL-3.

See [LICENSE](./LICENSE) file for details.

Copyright © 2025 Guardtime SA, guardtime.com
